package deployments

import (
	"bytes"
	"os"

	"github.com/mitchellh/mapstructure"
	"github.com/source-ag/ploy/engine"
	"gopkg.in/yaml.v3"
)

type Settings struct {
	PostDeployCommands [][]string `yaml:"post-deploy-commands,omitempty"`
}

type Config struct {
	Settings    Settings
	Deployments []engine.Deployment
}

// deploymentsYAML is the raw YAML shape, used only for marshaling/unmarshaling.
type deploymentsYAML struct {
	Settings    Settings         `yaml:"settings,omitempty"`
	Deployments []map[string]any `yaml:"deployments"`
}

func LoadConfigFromFile(deploymentsConfigPath string) (Config, error) {
	raw := &deploymentsYAML{}
	b, err := os.ReadFile(deploymentsConfigPath)
	if err != nil {
		return Config{}, err
	}
	if err = yaml.Unmarshal(b, raw); err != nil {
		return Config{}, err
	}
	deployments := make([]engine.Deployment, 0, len(raw.Deployments))
	for _, deployment := range raw.Deployments {
		deploymentEngine, err := engine.GetEngine(deployment["type"].(string))
		if err != nil {
			return Config{}, err
		}
		deploymentConfig := deploymentEngine.ResolveConfigStruct()
		if err = mapstructure.Decode(deployment, deploymentConfig); err != nil {
			return Config{}, err
		}
		deployments = append(deployments, deploymentConfig)
	}
	return Config{
		Settings:    raw.Settings,
		Deployments: deployments,
	}, nil
}

func WriteConfigToFile(deploymentsConfigPath string, cfg Config) error {
	deploymentMaps := make([]map[string]any, 0, len(cfg.Deployments))
	for _, deployment := range cfg.Deployments {
		var deploymentMap map[string]any
		if err := mapstructure.Decode(deployment, &deploymentMap); err != nil {
			return err
		}
		deploymentMaps = append(deploymentMaps, deploymentMap)
	}
	raw := deploymentsYAML{
		Settings:    cfg.Settings,
		Deployments: deploymentMaps,
	}
	return marshalYamlToFile(raw, deploymentsConfigPath)
}

// LoadDeploymentsFromFile is a compatibility shim; callers will migrate to LoadConfigFromFile.
func LoadDeploymentsFromFile(deploymentsConfigPath string) ([]engine.Deployment, error) {
	cfg, err := LoadConfigFromFile(deploymentsConfigPath)
	if err != nil {
		return nil, err
	}
	return cfg.Deployments, nil
}

// WriteDeploymentsToFile is a compatibility shim; callers will migrate to WriteConfigToFile.
func WriteDeploymentsToFile(deploymentsConfigPath string, deployments []engine.Deployment) error {
	return WriteConfigToFile(deploymentsConfigPath, Config{Deployments: deployments})
}

func marshalYamlToFile(in interface{}, path string) (err error) {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	defer func() {
		err = encoder.Close()
	}()
	encoder.SetIndent(2)
	err = encoder.Encode(in)
	if err != nil {
		return
	}
	err = os.WriteFile(path, buffer.Bytes(), 0644)
	return
}
