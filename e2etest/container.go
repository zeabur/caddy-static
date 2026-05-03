package e2etest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestCaddyContainer(t *testing.T) (c *testcontainers.DockerContainer, endpoint string) {
	t.Helper()
	exampleDir, err := filepath.Abs("../examples/caddy")
	if err != nil {
		t.Fatal(err)
	}
	return TestCaddyContainerWithFixture(t, exampleDir)
}

func TestCaddyContainerWithFixture(t *testing.T, fixtureDir string) (c *testcontainers.DockerContainer, endpoint string) {
	t.Helper()

	c, err := testcontainers.Run(context.Background(), "zeabur/caddy-static",
		testcontainers.CustomizeRequestOption(func(request *testcontainers.GenericContainerRequest) error {
			request.Files = append(request.Files, testcontainers.ContainerFile{
				HostFilePath:      fixtureDir,
				ContainerFilePath: "/usr/share/caddy",
				FileMode:          0o755,
			})
			return nil
		}),
		testcontainers.CustomizeRequestOption(func(request *testcontainers.GenericContainerRequest) error {
			request.WaitingFor = wait.ForLog("serving initial configuration")
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := c.Terminate(context.Background()); err != nil {
			t.Fatalf("Failed to terminate container: %v", err)
		}
	})

	endpoint, err = c.PortEndpoint(context.Background(), "8080/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}

	return c, endpoint
}
