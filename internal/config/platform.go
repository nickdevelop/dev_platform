// Package config defines the platform's runtime configuration types.
package config

// PlatformConfig is the platform-level runtime configuration: where the
// platform keeps its own data and which directories are authorized as
// workspace roots.
type PlatformConfig struct {
	// DataDir is the directory for the platform's own persisted data
	// (the SQLite database file). It must live outside every managed
	// project directory.
	DataDir string

	// WorkspaceRoots lists the directory paths the operator has
	// authorized as workspace roots for registering or creating projects.
	WorkspaceRoots []string
}
