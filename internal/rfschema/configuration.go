package rfschema

import (
	"errors"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
)

// Config hold the required parameters to create a instance of a Schema tree
type Config struct {
	// Required
	ContribDirPrefix  string
	SchemaDir         string
	MappingDir        string
	CrayRegistriesDir string
	DMTFRegistriesDir string
	Version           string

	// Optional Oem extensions
	OemMappings     string
	OemExtensionDir string
}

// SetupPFlagConfig will create a config structure that is tied to pflag flags
// This will create the required flags
func SetupPFlagConfig() *Config {
	config := Config{}

	// Configuration from command line flags
	flag.StringVar(&config.Version, "version", "", "Schema Version")
	flag.StringVar(&config.OemMappings, "oem", "", "Path to Oem mapping")
	flag.StringVar(&config.OemExtensionDir, "oemext", "", "Path to Oem extension directory")

	return &config
}

// ParsePFlagConfig will determine if the provided config has the required fields set
// when using pflags
func ParsePFlagConfig(config *Config) error {
	if config.Version == "" {
		return errors.New("-version flag is not set")
	}
	if config.OemMappings != "" && config.OemExtensionDir == "" {
		return errors.New("-oem flag required -oemext to be also set")
	}

	return nil
}

// SetupEnvConfig will pull the required schema tree creation parameters from the environment
func SetupEnvConfig() (*Config, error) {
	// Just need a location to where the `contrib` folder is.
	contribDirPrefix, ok := os.LookupEnv("CONTRIB_DIR_PREFIX")
	if !ok {
		contribDirPrefix = "."
	}

	versionStr, ok := os.LookupEnv("SCHEMA_VERSION")
	if !ok {
		return nil, errors.New("SCHEMA_VERSION environment variable is not set")
	}

	// Compute the schema and mapping directories by pre-pending the contrib dir prefix.
	schemaDir := filepath.Join(contribDirPrefix,
		"contrib/DMTF/DSP8010-Redfish_Schema/DSP8010_"+versionStr+"/json-schema")
	mappingDir := filepath.Join(contribDirPrefix, "contrib/Cray/hms-redfish-schema-tools/Mappings")

	// Check to make sure those directories are valid.
	if _, err := os.Stat(schemaDir); os.IsNotExist(err) {
		configError := errors.New("schema directory does not exist")
		log.WithField("schemaDir", schemaDir).Error(configError)
		return nil, configError
	} else {
		log.WithField("schemaDir", schemaDir).Info("Schema dir")
	}
	if _, err := os.Stat(mappingDir); os.IsNotExist(err) {
		configError := errors.New("mapping directory does not exist")
		logrus.WithField("mappingDir", mappingDir).Error(configError)
		return nil, configError
	} else {
		log.WithField("mappingDir", schemaDir).Info("Mapping dir")
	}

	oemMappings, oemOk := os.LookupEnv("OEM_MAPPINGS")
	oemExtensionDir, extOk := os.LookupEnv("OEM_EXTENSION_DIR")
	if oemOk && !extOk {
		return nil, errors.New("OEM_MAPPINGS environment variable requires OEM_EXTENSION_DIR to be also set")
	}

	return &Config{
		ContribDirPrefix: contribDirPrefix,
		SchemaDir:        schemaDir,
		MappingDir:       mappingDir,
		Version:          versionStr,
		OemMappings:      oemMappings,
		OemExtensionDir:  oemExtensionDir,
	}, nil
}

// LoadTreeFromConfigTree will construct the schema tree using the provided configuration
func LoadTreeFromConfig(config *Config) (*Tree, error) {
	err := GetVersionMapper().ReadDir(config.MappingDir)
	if err != nil {
		return nil, err
	}

	version, err := ParseVersion(config.Version)
	if err != nil {
		return nil, err
	}

	return CreateTree(config, version)
}
