package gosh

import "github.com/go-logr/logr"

// GlobalLog is the default logger to use if a command does not have one set.
// It defaults to a no-op logger
var GlobalLog logr.Logger

// CommandLogLevel is the verbosity level to log to when a command starts or exits
var CommandLogLevel = 5

// DebugLogLevel is the verbosity level to log to for internal debugging messages
var DebugLogLevel = 10
