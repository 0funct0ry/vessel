package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/0funct0ry/vessel/internal/api"
	"github.com/0funct0ry/vessel/internal/dockerapi"
	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/store/memstore"
	"github.com/0funct0ry/vessel/internal/store/sqlitestore"
	"github.com/0funct0ry/vessel/internal/version"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Vessel web server",
	Args:  cobra.NoArgs,
	RunE:  runServe,
}

func init() {
	addServeFlags(serveCmd.Flags())
}

// addServeFlags registers every `vessel serve` flag (SPEC §3.1) on the given
// flag set. It is called for both serveCmd and rootCmd, since running vessel
// with no subcommand is a shortcut for `vessel serve`. There are no
// persistent/inherited flags — each command owns its own local set.
func addServeFlags(flags *pflag.FlagSet) {
	addConfigFlag(flags)
	flags.String("addr", "127.0.0.1", "Bind address")
	flags.Int("port", 7373, "Bind port")
	flags.String("db", "", "SQLite file. Omitted means in-memory store")
	flags.String("docker-host", "unix:///var/run/docker.sock", "Docker Engine API host")
	flags.Bool("auth", false, "Require login. Implied when any user exists")
	flags.String("jwt-ttl", "24h", "Access-token lifetime")
	flags.Bool("read-only", false, "Serve GETs only; every mutating route returns 403")
	flags.String("base-path", "/", "Serve under a sub-path behind a proxy")
	flags.Bool("allow-exec", true, "Set false to remove the console entirely")
	flags.String("log-level", "info", "debug|info|warn|error")
	flags.Bool("open", false, "Open a browser on start")
	flags.Bool("i-know-what-im-doing", false, "Override the loopback-or-auth bind guard")
}

func runServe(cmd *cobra.Command, args []string) error {
	cfgFile, err := cmd.Flags().GetString("config")
	if err != nil {
		return err
	}

	v := newViper(cfgFile)
	if err := v.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	cfg, err := NewConfig(v)
	if err != nil {
		return err
	}

	if err := GuardBind(cfg.Addr, cfg.Auth, cfg.Override); err != nil {
		return err
	}
	dockerClient, err := dockerapi.New(cfg.DockerHost)
	if err != nil {
		return fmt.Errorf("create Docker client: %w", err)
	}
	var persistence store.Store
	if cfg.DB == "" {
		persistence = memstore.New()
	} else {
		persistence, err = sqlitestore.Open(cfg.DB)
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
	}
	defer persistence.Close()

	printBanner(cfg)

	router := api.NewRouter(api.Config{
		Docker:   dockerClient,
		ReadOnly: cfg.ReadOnly,
		BasePath: cfg.BasePath,
	})
	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port),
		Handler: router,
	}

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serveErr <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serveErr:
		return fmt.Errorf("server error: %w", err)
	case sig := <-sigCh:
		fmt.Printf("received %s, draining connections (up to 5s)...\n", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	fmt.Println("vessel stopped")
	return nil
}

func printBanner(cfg *Config) {
	fmt.Printf("Vessel %s — http://%s:%d\n", version.String(), cfg.Addr, cfg.Port)
	fmt.Printf("docker: %s (engine probe not yet implemented)\n", cfg.DockerHost)

	if cfg.DB == "" {
		fmt.Println("store:  in-memory (no --db given; webhooks and users will not persist)")
	} else {
		fmt.Printf("store:  sqlite: %s\n", cfg.DB)
	}

	authState := "off"
	if cfg.Auth {
		authState = "on"
	} else if isLoopback(cfg.Addr) {
		authState = "off (loopback bind)"
	} else if cfg.Override {
		authState = "off (bind guard overridden with --i-know-what-im-doing)"
	}
	fmt.Printf("auth:   %s\n", authState)

	if cfg.ReadOnly {
		fmt.Println("mode:   read-only")
	}
	if cfg.BasePath != "" && cfg.BasePath != "/" {
		fmt.Printf("base:   %s\n", strings.TrimSuffix(cfg.BasePath, "/"))
	}
}
