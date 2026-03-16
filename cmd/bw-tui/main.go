package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

func main() {
	repos, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	m := newModel(repos)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

// parseArgs resolves repos from CLI flags. With no flags, opens the current
// directory. --repo flags specify additional paths.
func parseArgs(args []string) ([]*RepoSource, error) {
	var paths []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--repo":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--repo requires a path argument")
			}
			i++
			paths = append(paths, args[i])
		case "--help", "-h":
			fmt.Println("Usage: bw-tui [--repo <path>]...")
			fmt.Println()
			fmt.Println("With no flags, opens the current directory.")
			fmt.Println("Use --repo to add repositories.")
			os.Exit(0)
		default:
			return nil, fmt.Errorf("unknown argument: %s", args[i])
		}
	}

	if len(paths) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		paths = []string{cwd}
	}

	var repos []*RepoSource
	for _, p := range paths {
		r, err := OpenRepo(p)
		if err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, nil
}
