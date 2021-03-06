package internal

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/coveooss/gotemplate/v3/collections"
	git "github.com/go-git/go-git/v5"
	cp "github.com/otiai10/copy"

	"github.com/AidanDelaney/scafall/pkg/internal/util"
)

// Present a local directory or a git repo as a Filesystem
func URLToFs(url string, subPath string, tmpDir string) (string, error) {
	// if the URL is a local folder, then do not git clone it
	if _, err := os.Stat(url); err == nil {
		cp.Copy(url, tmpDir)
	} else {
		_, err := git.PlainClone(tmpDir, false, &git.CloneOptions{
			URL:   url,
			Depth: 1,
		})
		if err != nil {
			return "", err
		}
	}

	requestedSubPath := path.Join(tmpDir, subPath)
	if _, err := os.Stat(requestedSubPath); err != nil {
		return "", fmt.Errorf("reequested subPath of template does not exist: %s", subPath)
	}
	return requestedSubPath, nil
}

func Create(inputDir string, overrides map[string]string, targetDir string) error {
	var values collections.IDictionary
	promptFile := filepath.Join(inputDir, PromptFile)

	// Create prompts and merge any overrides
	if _, err := os.Stat(promptFile); err == nil {
		prompts, err := ReadPromptFile(promptFile)
		if err != nil {
			return err
		}
		overridesDict := util.ToIDictionary(overrides)
		overridesFile := filepath.Join(inputDir, OverrideFile)
		if _, err := os.Stat(overridesFile); err == nil {
			os, err := ReadOverrides(overridesFile)
			overridesDict = overridesDict.Merge(os)
			if err != nil {
				return err
			}
		}

		values, err = AskPrompts(prompts, overridesDict, os.Stdin)
		if err != nil {
			return err
		}
		values = values.Merge(overridesDict)
	}

	errApply := Apply(inputDir, values, targetDir)
	if errApply != nil {
		return fmt.Errorf("failed to load new project skeleton: %s", errApply)
	}

	return nil
}
