// Scafall creates new source projects from project templates.  Project
// templates are stored in git repositories and new source projects are created
// on your local filesystem.
package scafall

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/coveooss/gotemplate/v3/collections"
	git "github.com/go-git/go-git/v5"
	cp "github.com/otiai10/copy"

	"github.com/AidanDelaney/scafall/pkg/internal"
	"github.com/AidanDelaney/scafall/pkg/internal/util"
)

// Scafall allows programmatic control over the default values for variables
// Overrides are skipped in prompts but can be locally overridden in a
// `.override.toml` file.
type Scafall struct {
	Overrides     map[string]string
	DefaultValues map[string]interface{}
	OutputFolder  string
}

type Option func(*Scafall)

func WithOutputFolder(folder string) Option {
	return func(s *Scafall) {
		s.OutputFolder = folder
	}
}

func WithOverrides(overrides map[string]string) Option {
	return func(s *Scafall) {
		s.Overrides = overrides
	}
}

func WithDefaultValues(defaults map[string]interface{}) Option {
	return func(s *Scafall) {
		s.DefaultValues = defaults
	}
}

// Create a new Scafall with the given options.
func NewScafall(opts ...Option) Scafall {
	var (
		defaultOverrides     = map[string]string{}
		defautlDefaultValues = map[string]interface{}{}
		defaultOutputFolder  = "."
	)

	s := Scafall{
		Overrides:     defaultOverrides,
		DefaultValues: defautlDefaultValues,
		OutputFolder:  defaultOutputFolder,
	}

	for _, opt := range opts {
		opt(&s)
	}

	return s
}

// Present a local directory or a git repo as a Filesystem
func urlToFs(url string, tmpDir string) (string, error) {
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

	return tmpDir, nil
}

// If there is no top level prompts and some subdirectories contain prompts,
// then we're dealing with a collection.  Otherwise it's scaffolding with no
// prompts
func isCollection(dir string) bool {
	promptFile := filepath.Join(dir, internal.PromptFile)
	if _, err := os.Stat(promptFile); err == nil {
		return false
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			promptFile := filepath.Join(dir, entry.Name(), internal.PromptFile)
			if _, err := os.Stat(promptFile); err == nil {
				return true
			}
		}
	}
	return false
}

func collection(s Scafall, inputDir string, outputDir string, prompt string) error {
	varName := "__ScaffoldUrl"
	vars := map[string]interface{}{}

	choices := []string{}
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != ".git" {
			choices = append(choices, entry.Name())
		}
	}

	initialPrompt := internal.Prompt{
		Name:     varName,
		Prompt:   prompt,
		Required: true,
		Choices:  choices,
	}
	prompts := internal.Prompts{
		Prompts: []internal.Prompt{initialPrompt},
	}
	promptFile := filepath.Join(inputDir, internal.OverrideFile)
	overrides, err := internal.ReadOverrides(promptFile)
	if err != nil {
		return err
	}
	overrides = overrides.Merge(util.ToIDictionary(s.Overrides))

	values, err := internal.AskPrompts(prompts, overrides, vars, os.Stdin)
	if err != nil {
		return err
	}
	if !values.Has(varName) {
		return fmt.Errorf("can not process the chosen element of collection: '%s'", varName)
	}
	choice := values.Get(varName).(string)
	targetProject := filepath.Join(inputDir, choice)
	return create(s, targetProject, outputDir)
}

// ScaffoldCollection creates a project after prompting the end-user to choose
// one of the projects in the collection at url.
func (s Scafall) ScaffoldCollection(url string, prompt string) error {
	tmpDir, _ := ioutil.TempDir("", "scafall")
	defer os.RemoveAll(tmpDir)

	inFs, err := urlToFs(url, tmpDir)
	if err != nil {
		return err
	}
	return collection(s, inFs, s.OutputFolder, prompt)
}

// Scaffold accepts url containing project templates and creates an output
// project.  The url can either point to a project template or a collection of
// project templates.
func (s Scafall) Scaffold(url string) error {
	tmpDir, _ := ioutil.TempDir("", "scafall")
	defer os.RemoveAll(tmpDir)

	inFs, err := urlToFs(url, tmpDir)
	if err != nil {
		return err
	}

	if isCollection(inFs) {
		return collection(s, inFs, s.OutputFolder, "Choose a project template")
	}
	return create(s, inFs, s.OutputFolder)
}

func create(s Scafall, inputDir string, targetDir string) error {
	var values collections.IDictionary
	promptFile := filepath.Join(inputDir, internal.PromptFile)

	// Create prompts and merge any overrides
	if _, err := os.Stat(promptFile); err == nil {
		prompts, err := internal.ReadPromptFile(promptFile)
		if err != nil {
			return err
		}
		overrides := util.ToIDictionary(s.Overrides)
		overridesFile := filepath.Join(inputDir, internal.OverrideFile)
		if _, err := os.Stat(overridesFile); err == nil {
			os, err := internal.ReadOverrides(overridesFile)
			overrides = overrides.Merge(overrides, os)
			if err != nil {
				return err
			}
		}

		values, err = internal.AskPrompts(prompts, overrides, s.DefaultValues, os.Stdin)
		if err != nil {
			return err
		}
		values = values.Merge(overrides)
	}

	errApply := internal.Apply(inputDir, values, s.OutputFolder)
	if errApply != nil {
		return fmt.Errorf("failed to load new project skeleton: %s", errApply)
	}

	return nil
}
