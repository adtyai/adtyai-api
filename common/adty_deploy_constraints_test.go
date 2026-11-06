package common

import (
	"os"
	"strings"
	"testing"
)

func TestDeploymentConstraintsAreInTheTestedPackages(t *testing.T) {
	mainSource, err := os.ReadFile("../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSource), `127.0.0.1:8005`) {
		t.Fatal("pprof listen address is not loopback")
	}
	for _, name := range []string{"../docker-compose.yml", "../docker-compose.dev.yml"} {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "123456") {
			t.Fatalf("%s publishes an example secret", name)
		}
	}
}
