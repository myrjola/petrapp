package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/myrjola/petrapp/internal/contexthelpers"
)

type BaseTemplateData struct {
	Authenticated bool
}

func newBaseTemplateData(r *http.Request) BaseTemplateData {
	return BaseTemplateData{
		Authenticated: contexthelpers.IsAuthenticated(r.Context()),
	}
}

// findModuleDir locates the directory containing the go.mod file.
func findModuleDir() (string, error) {
	var (
		dir string
		err error
	)
	dir, err = os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	for {
		if _, err = os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parentDir := filepath.Dir(dir)
		if parentDir == dir { // If we reached the root directory
			break
		}
		dir = parentDir
	}

	return "", os.ErrNotExist
}

// resolveAndVerifyTemplatePath resolves the template path and verifies it.
//
// If the templatePath is empty, it will attempt to find it from the module root.
func resolveAndVerifyTemplatePath(templatePath string) (string, error) {
	var err error
	if templatePath == "" {
		var modulePath string
		if modulePath, err = findModuleDir(); err != nil {
			return "", fmt.Errorf("find module dir: %w", err)
		}
		templatePath = filepath.Join(modulePath, "ui", "templates")
	}
	var stat os.FileInfo
	if stat, err = os.Stat(templatePath); err != nil {
		return "", fmt.Errorf("template path not found %s: %w", templatePath, err)
	}
	if !stat.IsDir() {
		return "", fmt.Errorf("template path is not a directory: %s", templatePath)
	}
	return templatePath, nil
}
