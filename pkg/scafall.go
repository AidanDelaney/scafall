package scafall

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
)

type Scafall struct {
	Variables map[string]interface{}
	Reserved  []string
}

func New(vars map[string]interface{}, reservedPromptValues []string) Scafall {
	return Scafall{
		Variables: vars,
		Reserved:  reservedPromptValues,
	}
}

func urlToFs(url string) (billy.Filesystem, error) {
	var inFs billy.Filesystem

	// if the URL is a local folder, then do not git clone it
	if _, err := os.Stat(url); err == nil {
		inFs = osfs.New(url)
	} else {
		inFs = memfs.New()
		_, err := git.Clone(memory.NewStorage(), inFs, &git.CloneOptions{
			URL:   url,
			Depth: 1,
		})
		if err != nil {
			return nil, err
		}
	}

	return inFs, nil
}

// If there is no top level prompts and some subdirectories contain prompts,
// then we're dealing with a collection.  Otherwise it's scaffolding with no
// prompts
func isCollection(bfs billy.Filesystem) bool {
	if _, err := bfs.Stat(PromptFile); err == nil {
		return false
	}

	entries, err := bfs.ReadDir("/")
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if entry.IsDir() {
			promptFile := filepath.Join(entry.Name(), PromptFile)
			if _, err := bfs.Stat(promptFile); err == nil {
				return true
			}
		}
	}
	return false
}

func collection(s Scafall, inFs billy.Filesystem, outputDir string, prompt string) error {
	varName := "__ScaffoldUrl"
	vars := map[string]interface{}{}

	choices := []string{}
	entries, err := inFs.ReadDir("/")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			choices = append(choices, entry.Name())
		}
	}

	initialPrompt := Prompt{
		Name:     varName,
		Prompt:   prompt,
		Required: true,
		Choices:  choices,
	}
	prompts := Prompts{
		Prompts: []Prompt{initialPrompt},
	}
	overrides, err := readOverrides(inFs, OverrideFile)
	if err != nil {
		return err
	}

	askPrompts(&prompts, vars, overrides)
	if _, exists := vars[varName]; !exists {
		return fmt.Errorf("can not process the chosen element of collection: '%s'", varName)
	}
	choice := vars[varName].(string)
	inFs, err = inFs.Chroot(choice)
	if err != nil {
		return nil
	}
	return create(s, inFs, outputDir)
}

func (s Scafall) ScaffoldCollection(url string, prompt string, outputDir string) error {
	inFs, err := urlToFs(url)
	if err != nil {
		return err
	}
	return collection(s, inFs, outputDir, prompt)
}

func (s Scafall) Scaffold(url string, outputDir string) error {
	inFs, err := urlToFs(url)
	if err != nil {
		return err
	}

	if isCollection(inFs) {
		return collection(s, inFs, outputDir, "Choose a project template")
	}
	return create(s, inFs, outputDir)
}
