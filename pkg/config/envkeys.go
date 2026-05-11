package config

// Runtime environment variable keys for OneAppFactory.
const (
	// EnvHome overrides the base directory for OneAppFactory data.
	// Default: ~/.appfactory
	EnvHome = "ONEAPPFACTORY_HOME"

	// EnvConfig overrides the full path to the JSON config file.
	// Default: $ONEAPPFACTORY_HOME/config.json
	EnvConfig = "ONEAPPFACTORY_CONFIG"
)
