// Scafall creates new source projects from project templates.  Project
// templates are stored in git repositories and new source projects are created
// on your local filesystem.
package scafall

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/AidanDelaney/scafall/pkg/internal"
)

// Scafall allows programmatic control over the default values for variables
// Overrides are skipped in prompts but can be locally overridden in a
// `.override.toml` file.
type Scafall struct {
	URL          string
	Arguments    map[string]string
	OutputFolder string
	SubPath      string
	TmpDir       string
}

type Option func(*Scafall)

func WithOutputFolder(folder string) Option {
	return func(s *Scafall) {
		s.OutputFolder = folder
	}
}

func WithArguments(arguments map[string]string) Option {
	return func(s *Scafall) {
		s.Arguments = arguments
	}
}

func WithSubPath(subPath string) Option {
	return func(s *Scafall) {
		s.SubPath = subPath
	}
}

func WithTmpDir(tmpDir string) Option {
	return func(s *Scafall) {
		s.TmpDir = tmpDir
	}
}

// Create a new Scafall with the given options.
func NewScafall(url string, opts ...Option) (Scafall, error) {
	var (
		defaultArguments    = map[string]string{}
		defaultOutputFolder = "."
	)

	s := Scafall{
		URL:          url,
		Arguments:    defaultArguments,
		OutputFolder: defaultOutputFolder,
	}

	for _, opt := range opts {
		opt(&s)
	}

	if s.TmpDir == "" {
		tmpDir, err := os.MkdirTemp("", "scafall")
		if err != nil {
			return Scafall{}, err
		}
		s.TmpDir = tmpDir
	}

	return s, nil
}

func clone(s Scafall) (string, error) {
	fs, err := internal.URLToFs(s.URL, s.SubPath, s.TmpDir)
	if err != nil {
		return "", err
	}
	return fs, err
}

// Scaffold accepts url containing project templates and creates an output
// project.  The url can either point to a project template or a collection of
// project templates.
func (s Scafall) Scaffold() error {
	inFs, err := clone(s)
	if err != nil {
		return err
	}

	if isCollection, choices := internal.IsCollection(inFs); isCollection {
		template, err := internal.AskQuestion("choose a project template", choices, os.Stdin)
		if err != nil {
			return err
		}
		inFs = path.Join(inFs, template)
	}

	return internal.Create(inFs, s.Arguments, s.OutputFolder)
}

// Arguments returns a list of variable names that can be passed to the template
func (s Scafall) TemplateArguments() (string, []string, error) {
	inFs, err := clone(s)
	if err != nil {
		return "", nil, err
	}

	if isCollection, choices := internal.IsCollection(inFs); isCollection {
		return "templates available in collection", choices, nil
	}

	promptFile := filepath.Join(inFs, internal.PromptFile)
	ps, err := internal.ReadPromptFile(promptFile)
	if err != nil {
		return "could not detect valid prompts", nil, err
	}
	argsStrings := make([]string, len(ps.Prompts))
	for i, p := range ps.Prompts {
		if len(p.Choices) == 0 {
			argsStrings[i] = fmt.Sprintf("%s (default: %s)", p.Name, p.Default)
		} else {
			cString := strings.Join(p.Choices, ", ")
			argsStrings[i] = fmt.Sprintf("%s=%s (default: %s)", p.Name, cString, p.Choices[0])
		}
	}
	return "arguments offered by template", argsStrings, nil
}
