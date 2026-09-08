package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check whether Vessel can reach the Docker engine",
	Args:  cobra.NoArgs,
	RunE:  runDoctor,
}

func init() {
	flags := doctorCmd.Flags()
	addConfigFlag(flags)
	flags.String("docker-host", "", "Docker Engine API host (auto-detected if omitted)")
}

// runDoctor resolves the configured Docker host, dials it, and reports
// engine version, negotiated API version, and socket permissions, with a
// remedy line per failure.
func runDoctor(cmd *cobra.Command, args []string) error {
	cfgFile, err := cmd.Flags().GetString("config")
	if err != nil {
		return err
	}

	v := newViper(cfgFile)
	if err := v.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	resolved := detectDockerHost(v.GetString("docker-host"))
	host := resolved.Host
	fmt.Printf("docker-host: %s%s\n", host, dockerHostBannerNote(resolved.Source))

	if strings.HasPrefix(host, "unix://") {
		checkSocketPermissions(strings.TrimPrefix(host, "unix://"))
	}

	client, err := dockerapi.New(host)
	if err != nil {
		fmt.Printf("  %v\n", err)
		fmt.Println("  remedy: --docker-host must start with unix://, tcp:// or tcps://")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		reportEngineError(err)
		return nil
	}

	info, err := client.Version(ctx)
	if err != nil {
		reportEngineError(err)
		return nil
	}

	fmt.Println("  engine reachable")
	fmt.Printf("  engine version: %s (%s/%s, kernel %s)\n", info.Version, info.Os, info.Arch, info.KernelVer)
	fmt.Printf("  api version: %s (negotiated: %s)\n", info.APIVersion, client.APIVersion())

	return nil
}

// checkSocketPermissions performs the M1-era existence/readability check on
// a unix socket path, ahead of the real engine dial.
func checkSocketPermissions(path string) {
	info, err := os.Stat(path)
	if err != nil {
		fmt.Printf("  socket not found: %v\n", err)
		fmt.Println("  remedy: is the Docker daemon running, and does this path match its socket?")
		return
	}

	if info.Mode()&os.ModeSocket == 0 {
		fmt.Printf("  %s exists but is not a socket\n", path)
		return
	}

	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		fmt.Printf("  socket found but not accessible: %v\n", err)
		fmt.Println("  remedy: is this user in the docker group? try: sudo usermod -aG docker $USER")
		return
	}
	f.Close()

	fmt.Println("  socket found and readable")
}

// reportEngineError prints the dial/ping/version error along with a remedy
// line appropriate to its category.
func reportEngineError(err error) {
	fmt.Printf("  engine unreachable: %v\n", err)

	switch {
	case errors.Is(err, dockerapi.ErrUnreachable):
		fmt.Println("  remedy: is the Docker daemon running and is --docker-host correct?")
	default:
		var apiErr *dockerapi.APIError
		if errors.As(err, &apiErr) {
			fmt.Printf("  remedy: engine returned HTTP %d — check the daemon logs\n", apiErr.Status)
			return
		}
		fmt.Println("  remedy: check --docker-host and that the Docker daemon is running")
	}
}
