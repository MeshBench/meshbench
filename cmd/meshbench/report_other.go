//go:build !windows

package main

// Every other platform keeps its standard handles, so a message written to
// stderr reaches whoever ran the command and there is nothing to adopt or to
// write down.

func adoptConsole() {}

func reportFatal(string) string { return "" }
