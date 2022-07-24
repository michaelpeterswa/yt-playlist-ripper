package settings

import (
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

type Settings struct {
	Playlists []string `json:"playlists" yaml:"playlists"`
}

func InitSettings() (*Settings, error) {
	var settings *Settings
	yamlFile, err := ioutil.ReadFile("/config/config.yaml")
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal([]byte(yamlFile), &settings)
	if err != nil {
		return nil, err
	}
	return settings, nil
}
