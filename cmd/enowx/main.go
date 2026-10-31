// Command enowx is the enowX desktop app.
//
//	enowx                            open this machine (starting its daemon)
//	enowx --remote user@host[:port]  open a server over SSH
//	enowx --project DIR              also open DIR as a project
//	enowx daemon                     run this machine's daemon (started by the app)
//	enowx hook STATE                 report an agent's state (run by its hooks)
//	enowx ls|spawn|send|wait|…       the command line (enowx help)
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"github.com/enowx/enowx/cli"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/transport"
	enowxui "github.com/enowx/enowx/ui"
)

func main() {
	if err := daemon.MoveOldHome(); err != nil {
		fmt.Fprintln(os.Stderr, "enowx:", err)
	}
	if len(os.Args) > 1 && os.Args[1] == "daemon" {
		if err := daemon.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "enowx daemon:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "hook" {
		os.Exit(daemon.Hook(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}
	if cli.IsDockerHelper(os.Args[0]) {
		if err := cli.DockerHelper(os.Args[1:], os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "enowx:", err)
			os.Exit(1)
		}
		return
	}
	if cli.Is(os.Args[1:]) {
		if err := cli.Run(os.Args[1:], os.Stdout); err != nil {
			var exit cli.ExitError
			if !errors.As(err, &exit) {
				fmt.Fprintln(os.Stderr, "enowx:", err)
			}
			os.Exit(cli.Exit(err))
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "display" {
		if err := daemon.Display(); err != nil {
			fmt.Fprintln(os.Stderr, "enowx display:", err)
			os.Exit(1)
		}
		return
	}

	remote := flag.String("remote", "", "open the server at `user@host[:port]` over SSH")
	project := flag.String("project", "", "open `DIR` (a path on that host) as a project")
	flag.Parse()

	dir, err := transport.Dir()
	if err != nil {
		log.Fatal(err)
	}
	store, err := hosts.Open(filepath.Join(dir, "hosts.json"))
	if err != nil {
		log.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	start := enowxui.LocalHost()
	if *remote != "" {
		start = hosts.Host{ID: "cli", Name: *remote, Address: *remote}
		for _, h := range store.List() {
			if h.Address == *remote {
				start = h
			}
		}
	}
	app := enowxui.New(enowxui.Options{
		Hosts:         store,
		Start:         start,
		DaemonCommand: []string{daemonExe(exe, dir), "daemon"},
		OpenProject:   *project,
		StateFile:     filepath.Join(dir, "app.json"),
		ResumeHost:    *remote == "",
	})
	mygo.App.WhenReady(func() { app.Open() })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
