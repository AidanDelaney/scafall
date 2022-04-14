package scafall

import (
	"io"
	"os"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
)

type Scafall struct {
	Variables map[string]interface{}
	Reserved  []string
	Stdin     io.ReadCloser
}

func New(vars map[string]interface{}, reservedPromptValues []string) Scafall {
	return Scafall{
		Variables: vars,
		Reserved:  reservedPromptValues,
		Stdin:     os.Stdin,
	}
}

func (s Scafall) ScaffoldCollection(urls map[string]string, prompt string, outputDir string) error {
	varName := "__ScaffoldUrl"
	vars := map[string]interface{}{}
	choices := make([]string, 0, len(urls))
	for k := range urls {
		choices = append(choices, k)
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
	askPrompts(s.Stdin, &prompts, vars, map[string]string{})
	url := vars[varName].(string)
	return s.Scaffold(urls[url], outputDir)
}

func (s Scafall) Scaffold(url string, outputDir string) error {
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
			return err
		}
	}

	return create(s, inFs, outputDir)
}
