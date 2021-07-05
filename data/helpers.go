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
