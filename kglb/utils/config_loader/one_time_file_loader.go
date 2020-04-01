package config_loader

import (
	"io/ioutil"

	"godropbox/errors"
)

type OneTimeFileLoader struct {
	path string

	updateChan chan interface{}
}

var _ ConfigLoader = &OneTimeFileLoader{}

func NewOneTimeFileLoader(provider ConfigProvider, path string) (*OneTimeFileLoader, error) {
	loader := &OneTimeFileLoader{
		path:       path,
		updateChan: make(chan interface{}, 1),
	}

	if content, err := ioutil.ReadFile(path); err != nil {
		return nil, errors.Wrapf(err, "fails to read '%s' file: ", path)
	} else {
		// validate config.
		cfg, err := provider.Parse(content)
		if err != nil {
			return nil, err
		}
		if err = provider.Validate(cfg); err != nil {
			return nil, err
		}
		loader.updateChan <- cfg
	}

	return loader, nil
}

// Config updates channel.
func (l *OneTimeFileLoader) Updates() <-chan interface{} {
	return l.updateChan
}

// Stopping config loader and closing its update channel.
func (l *OneTimeFileLoader) Stop() {
	close(l.updateChan)
}
