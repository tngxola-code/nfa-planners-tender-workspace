//go:build integration

// Package integration exercises the auth chain against a real Keycloak
// running the committed realm. Unit tests build Claims by hand and so
// cannot catch a realm that issues tokens missing sub, or a scope mapping
// that resolves to nothing. This suite can.
package integration

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const realmName = "nfa"

// issuer is the realm issuer every test builds URLs from.
var issuer string

func TestMain(m *testing.M) {
	// Opt-in fast path: point at an already-running Keycloak (the compose
	// stack) instead of starting one. CI never sets this.
	if base := os.Getenv("NFA_TEST_KEYCLOAK_URL"); base != "" {
		issuer = base + "/realms/" + realmName
		log.Printf("integration: using existing keycloak at %s", issuer)
		os.Exit(m.Run())
	}

	ctx := context.Background()
	realmPath, err := filepath.Abs("../../../deploy/keycloak/realm.json")
	if err != nil {
		log.Fatalf("locate realm.json: %v", err)
	}
	if _, err := os.Stat(realmPath); err != nil {
		log.Fatalf("realm.json not found at %s: %v", realmPath, err)
	}

	req := testcontainers.ContainerRequest{
		Image:        "quay.io/keycloak/keycloak:26.0",
		ExposedPorts: []string{"8080/tcp"},
		Cmd:          []string{"start-dev", "--import-realm", "--hostname-strict=false"},
		Env: map[string]string{
			// Keycloak 26 names. The compose file still uses the
			// deprecated KEYCLOAK_ADMIN pair.
			"KC_BOOTSTRAP_ADMIN_USERNAME": "admin",
			"KC_BOOTSTRAP_ADMIN_PASSWORD": "admin",
		},
		Files: []testcontainers.ContainerFile{{
			HostFilePath:      realmPath,
			ContainerFilePath: "/opt/keycloak/data/import/realm.json",
			FileMode:          0o444,
		}},
		WaitingFor: wait.ForHTTP("/realms/" + realmName + "/.well-known/openid-configuration").
			WithPort("8080/tcp").
			WithStartupTimeout(3 * time.Minute),
	}

	kc, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		log.Fatalf("start keycloak: %v", err)
	}

	host, err := kc.Host(ctx)
	if err != nil {
		log.Fatalf("container host: %v", err)
	}
	port, err := kc.MappedPort(ctx, "8080")
	if err != nil {
		log.Fatalf("mapped port: %v", err)
	}
	issuer = fmt.Sprintf("http://%s:%s/realms/%s", host, port.Port(), realmName)
	log.Printf("integration: keycloak ready at %s", issuer)

	code := m.Run()

	if err := kc.Terminate(context.Background()); err != nil {
		log.Printf("terminate keycloak: %v", err)
	}
	os.Exit(code)
}
