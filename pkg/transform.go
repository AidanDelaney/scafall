package scafall

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Masterminds/sprig/v3"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/manifoldco/promptui"
)

const (
	PromptFile   string = "prompts.toml"
	OverrideFile string = ".override.toml"
)

var (
	ReservedPromptVariables = []string{}
	IgnoredNames            = []string{"/" + PromptFile, "/" + OverrideFile, "/.git"}
)

type Prompt struct {
	Name     string   `toml:"name" binding:"required"`
	Prompt   string   `toml:"prompt" binding:"required"`
	Required bool     `toml:"required"`
	Default  string   `toml:"default"`
	Choices  []string `toml:"choices,omitempty"`
}

type Prompts struct {
	Prompts []Prompt `toml:"prompt"`
}

func requireNonEmptyString(s string) error {
	if s == "" {
		return errors.New("please provide a non-empty value")
	}
	return nil
}

func requireId(s string) error {
	return nil
}

func askPrompts(stdin io.ReadCloser, prompts *Prompts, vars map[string]interface{}, overides map[string]string) error {
	for _, prompt := range prompts.Prompts {
		if overide, exists := overides[prompt.Name]; exists {
			vars[prompt.Name] = overide
		}

		var result string
		var err error

		if prompt.Choices == nil || len(prompt.Choices) == 0 {
			var validateFunc promptui.ValidateFunc = requireId
			if prompt.Required {
				validateFunc = requireNonEmptyString
			}
			p := promptui.Prompt{
				Label:    prompt.Prompt,
				Default:  prompt.Default,
				Validate: validateFunc,
				Stdin:    stdin,
			}
			result, err = p.Run()
		} else {
			p := promptui.Select{
				Label: prompt.Prompt,
				Items: prompt.Choices,
				Stdin: stdin,
			}
			_, result, err = p.Run()
		}
		if err == nil {
			vars[prompt.Name] = result
		}
	}
	return nil
}

func readFile(bfs billy.Filesystem, name string) (string, error) {
	file, err := bfs.Open(name)
	if err != nil {
		return "", fmt.Errorf("cannot open file %s", name)
	}
	defer file.Close()

	buf, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("cannot read file %s", name)
	}
	return string(buf), nil
}

func contains(strings []string, element string) bool {
	for _, s := range strings {
		if s == element {
			return true
		}
	}
	return false
}

func readPromptFile(bfs billy.Filesystem, name string) (*Prompts, error) {
	promptData, err := readFile(bfs, name)
	if err != nil {
		return nil, err
	}

	prompts := Prompts{}
	if _, err := toml.Decode(promptData, &prompts); err != nil {
		return nil, fmt.Errorf("%s file does not match required format: %s", name, err)
	}

	for _, prompt := range prompts.Prompts {
		if contains(ReservedPromptVariables, prompt.Name) {
			return nil, fmt.Errorf("%s file contains reserved variable: %s", name, prompt.Name)
		}
	}

	return &prompts, nil
}

func readOverrides(bfs billy.Filesystem, name string) (map[string]string, error) {
	overrides := map[string]string{}

	overrideData, err := readFile(bfs, name)
	if err != nil {
		return nil, err
	}

	if _, err := toml.Decode(overrideData, &overrides); err != nil {
		return nil, fmt.Errorf("%s file does not match required format: %s", name, err)
	}

	for k, _ := range overrides {
		if contains(ReservedPromptVariables, k) {
			return nil, fmt.Errorf("%s file contains reserved variable: %s", name, k)
		}
	}

	return overrides, nil
}

func isPrefixOf(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func apply(bfs billy.Filesystem, vars map[string]interface{}) (billy.Filesystem, error) {
	outFs := memfs.New()

	err := Walk(bfs, "/", func(path string, info fs.FileInfo, err error) error {
		// Do not write the prompt file to the output project
		if isPrefixOf(path, IgnoredNames) {
			return nil
		}

		t, terr := transform(&vars, path)
		if terr != nil {
			return nil
		}
		tpath := string(t)

		// Checking, if embedded file is a folder.
		if info.IsDir() {
			// Create folders structure from embedded.
			if err := outFs.MkdirAll(tpath, 0755); err != nil {
				return err
			}
		}

		// Checking, if embedded file is not a folder.
		if !info.IsDir() {
			// Set file data.
			fileData, errReadFile := readFile(bfs, path)
			if errReadFile != nil {
				return errReadFile
			}

			transformed, tfErr := transform(&vars, fileData)
			if tfErr != nil {
				return fmt.Errorf("failed to subsitute variables in %s", tpath)
			}
			// Create file from embedded.
			if fileInfo, err := outFs.OpenFile(tpath, os.O_CREATE|os.O_RDWR, info.Mode()); err == nil {
				defer fileInfo.Close()
				if _, err := fileInfo.Write(transformed); err != nil {
					return err
				}
			} else {
				return err
			}
		}

		return nil
	})

	return outFs, err
}

func copy(inFs billy.Filesystem, outFs billy.Filesystem) error {
	err := Walk(inFs, "/", func(path string, info fs.FileInfo, err error) error {
		// Checking, if embedded file is a folder.
		if info.IsDir() {
			// Create folders structure from embedded.
			if err := outFs.MkdirAll(path, 0755); err != nil {
				return err
			}
		}

		// Checking, if embedded file is not a folder.
		if !info.IsDir() {
			// create a copy
			outFile, errCreateFile := outFs.OpenFile(path, os.O_CREATE|os.O_RDWR, info.Mode())
			if errCreateFile != nil {
				return fmt.Errorf("failed to create file: %s %s", path, err)
			}
			defer outFile.Close()

			inFile, errOpen := inFs.Open(path)
			if errOpen != nil {
				return fmt.Errorf("failed to copy file: %s %s", path, err)
			}
			defer inFile.Close()

			if n, errCopy := io.Copy(outFile, inFile); errCopy != nil {
				return fmt.Errorf("failed to write data to file: %s %v (%d bytes)", path, err, n)
			}
			log.Default().Printf("    %s  %s", "create", path)
		}

		return nil
	})
	return err
}

func create(s Scafall, bfs billy.Filesystem, targetDir string) error {
	// don't clobber any existing files
	if _, ok := os.Stat(targetDir); ok == nil {
		return fmt.Errorf("directory %s already exists", targetDir)
	}

	var transformedFs = bfs

	if _, err := bfs.Stat(PromptFile); err == nil {
		prompts, err := readPromptFile(bfs, PromptFile)
		if err != nil {
			return err
		}
		overides := map[string]string{}
		if _, err := bfs.Stat(OverrideFile); err == nil {
			overides, err = readOverrides(bfs, OverrideFile)
			if err != nil {
				return err
			}
		}
		err = askPrompts(s.Stdin, prompts, s.Variables, overides)
		if err != nil {
			return err
		}
	}

	transformedFs, err := apply(bfs, s.Variables)
	if err != nil {
		return err
	}

	os.MkdirAll(targetDir, 0755)
	outFs := osfs.New(targetDir)
	err = copy(transformedFs, outFs)
	if err != nil {
		return fmt.Errorf("failed to load new project skeleton: %s", err)
	}

	return nil
}

func transform(env *map[string]interface{}, data string) ([]byte, error) {
	var output bytes.Buffer
	tpl, err := template.New("bp").Funcs(sprig.FuncMap()).Parse(data)
	if err != nil {
		return nil, errors.New("cannot parse file template")
	}
	err = tpl.Execute(&output, *env)
	if err != nil {
		return nil, errors.New("cannot replace variables in file template")
	}
	return output.Bytes(), err
}
