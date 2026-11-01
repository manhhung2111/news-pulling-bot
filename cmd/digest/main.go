// Command digest builds and posts the daily AI engineering digest.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mhung2111/ai-eng-digest/internal/config"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	dryRun := flag.Bool("dry-run", false, "print the digest instead of posting it")
	flag.Parse()

	if _, err := config.Load(*cfgPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	_ = dryRun
}
