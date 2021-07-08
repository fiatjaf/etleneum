package data

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
)

func readJSON(path string, out interface{}) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading json file %s: %w", path, err)
	}
	err = json.Unmarshal(data, out)
	if err != nil {
		return fmt.Errorf("error unmarshaling json file %s: %w", path, err)
	}

	return nil
}

func writeFile(path string, contents []byte) error {
	if err := ioutil.WriteFile(path, contents, 0644); err != nil {
		return err
	}

	if err := gitAdd(path); err != nil {
		return err
	}

	return nil
}

func writeJSON(path string, contents interface{}) error {
	if payloadJSON, err := json.MarshalIndent(contents, "", "  "); err != nil {
		return err
	} else if err := writeFile(path, payloadJSON); err != nil {
		return err
	}

	return nil
}
