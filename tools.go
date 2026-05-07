package main

import "os"

func readFile(filepath string) (string, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
