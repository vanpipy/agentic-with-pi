// Package log implements awp's logging facade on top of log/slog.
//
// The package is organised around two capabilities:
//
//  1. Printing — emitting log records to configured sinks (slog handler
//     setup). Owns Config, DefaultConfig, Setup, and the file-open
//     helper. Reads path layout from internal/paths.
//
//  2. Lifecycle — managing the log file on disk: rotate by size, prune
//     by age. Owns Rotate and CleanupOld. These are pure file
//     operations; they take paths as arguments and do not touch env
//     or path resolution, so they can be reused by other packages.
//
// Setup is a composition entry point: it prepares the log directory
// (paths.EnsureDir), prunes old files (CleanupOld), rotates the
// current file (Rotate), and then installs the slog handlers. Callers
// that need finer control over either capability can call the
// individual functions directly.
//
// # Path resolution
//
// DefaultLogFile delegates Home() to internal/storage, which is the
// canonical path resolver in this codebase.
//
// # Layout
//
//   - setup.go     — printing: Config, Setup, DefaultConfig, openAppend,
//     DefaultLogFile.
//   - lifecycle.go — file lifecycle: Rotate, CleanupOld.
package log
