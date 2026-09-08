package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

func TestExecRouteRegistration(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
		want int
	}{
		{"disabled", Config{Docker: newFakeDockerClient()}, http.StatusNotFound},
		{"read only", Config{Docker: newFakeDockerClient(), AllowExec: true, ReadOnly: true}, http.StatusNotFound},
		{"enabled rejects missing origin", Config{Docker: newFakeDockerClient(), AllowExec: true}, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRouter(tc.cfg)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c1/exec", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSameOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://vessel.test/api/v1/containers/c1/exec", nil)
	req.Host = "vessel.test"
	req.Header.Set("Origin", "https://vessel.test")
	if !sameOrigin(req) {
		t.Fatal("matching origin rejected")
	}
	req.Header.Set("Origin", "https://other.test")
	if sameOrigin(req) {
		t.Fatal("cross origin accepted")
	}
	req.Header.Del("Origin")
	if sameOrigin(req) {
		t.Fatal("missing origin accepted")
	}
}

func TestCreateExecFallsBackOnlyForMissingShell(t *testing.T) {
	fake := newFakeDockerClient()
	fake.execCreate = func(opts dockerapi.ExecOptions) (string, error) {
		if opts.Cmd[0] == "/custom/sh" {
			return "", errors.New("dockerapi: 500: exec: \"/custom/sh\": no such file or directory")
		}
		return "exec1", nil
	}
	s := &server{docker: fake}
	id, command, fallback, err := s.createExec(context.Background(), "c1", []string{"/custom/sh"}, "", true)
	if err != nil || id != "exec1" || command != "/bin/bash" || fallback != "/bin/bash" {
		t.Fatalf("id=%q command=%q fallback=%q err=%v", id, command, fallback, err)
	}
	if len(fake.execCalls) != 2 {
		t.Fatalf("create calls=%d", len(fake.execCalls))
	}

	fake.execCalls = nil
	fake.execCreate = func(dockerapi.ExecOptions) (string, error) {
		return "", errors.New("dockerapi: 500: permission denied")
	}
	_, _, _, err = s.createExec(context.Background(), "c1", []string{"/custom/sh"}, "", true)
	if err == nil || len(fake.execCalls) != 1 {
		t.Fatalf("unexpected fallback: calls=%d err=%v", len(fake.execCalls), err)
	}
}

func TestExecCapabilityTracksRouteAvailability(t *testing.T) {
	s := &server{execOn: false}
	if _, ok := s.capabilities("admin")["containers.exec"]; ok {
		t.Fatal("disabled console advertised as capable")
	}
	s.execOn = true
	if !s.capabilities("admin")["containers.exec"] {
		t.Fatal("enabled console missing capability")
	}
}
