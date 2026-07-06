package main

import "fmt"

func printUsage() {
	fmt.Println(`memsh records shell commands and returns ranked suggestions.

Usage:
  memsh
	Open the interactive command search box

  memsh help
	Show this help text

  memsh settings
	List configurable memsh settings and current values

  memsh settings set MEMSH_MAX_SUGGESTIONS 10
	Persist a memsh setting under ~/.config/memsh/settings.zsh

  memsh settings set MEMSH_ENABLE_DIRECTORY_AWARENESS 1
	Rank commands used in the current directory first (unset to disable)

  memsh pick --query "git"
	Open the interactive picker with a pre-filled query

  memsh delete "git status"
	Delete an exact command from the suggestion database

  memsh clear
	Prune the least-used 10% of stored commands

  memsh destroy
	Destroy all stored commands

  memsh record --command "git status" --directory "$PWD" --exit-code 0
	Record a successful command

  memsh search --query "git" --limit 5
	Search for up to 5 ranked suggestions

  memsh stats
	Show stored command stats

  memsh doctor
	Show resolved paths and DB health

  memsh version
	Show memsh version`)
}
