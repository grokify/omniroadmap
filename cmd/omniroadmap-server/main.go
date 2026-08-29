// Command omniroadmap-server runs the DashForge analytics engine composed
// with the OmniRoadmap connector — the pattern DashForge's ADR-0001
// prescribes: the engine core ships no connectors; application binaries
// compose engine + connector.
//
// Bring-up with a local OmniRoadmap dolt sql-server:
//
//	export OMNIROADMAP_DSN='root:@tcp(127.0.0.1:13307)/omniroadmap'
//	go run ./cmd/omniroadmap-server serve --address 127.0.0.1:13319 --enable-ollama
//
// then register the source once (it persists in .dashforge/ relative to the
// working directory, or the metadata DB when --db-url is set):
//
//	curl -X POST http://127.0.0.1:13319/api/v1/analytics/sources \
//	  -H 'Content-Type: application/json' \
//	  -d '{"id":"omniroadmap","name":"OmniRoadmap","connector":"omniroadmap",
//	       "dsnRef":"env://OMNIROADMAP_DSN","enabled":true}'
package main

import (
	"os"

	"github.com/plexusone/dashforge/cmd/dashforge-server/cmd"

	// Register the OmniRoadmap connector with the DashForge engine.
	_ "github.com/grokify/omniroadmap/dashforgeconnector"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
