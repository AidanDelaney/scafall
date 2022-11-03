package internal_test

import (
	"os"
	"path/filepath"
	"testing"

	h "github.com/buildpacks/pack/testhelpers"
	"github.com/buildpacks/scafall/pkg/internal"
	"github.com/sclevine/spec"
)

func testCollection(t *testing.T, when spec.G, it spec.S) {
	type TestCase struct {
		title     string
		folders   []string
		templates []string
	}
	testCases := []TestCase{
		{"all folders", []string{"option1", "option2"}, []string{"option1", "option2"}},
		{"some folders", []string{"option1", "option2", "option3"}, []string{"option3"}},
	}

	for _, testCase := range testCases {
		testCase := testCase
		when("Reading a filesystem", func() {
			var (
				collectionDir *string
			)
			it.Before(func() {
				tmpDir, err := os.MkdirTemp("", "scafall")
				h.AssertNil(t, err)
				collectionDir = &tmpDir
				for _, folder := range testCase.folders {
					d := filepath.Join(*collectionDir, folder)
					os.Mkdir(d, 0700)
				}

				for _, folder := range testCase.templates {
					f := filepath.Join(*collectionDir, folder, "prompts.toml")
					os.WriteFile(f, []byte{}, 0400)
				}
			})
			it.After(func() {
				os.RemoveAll(*collectionDir)
			})
			it("detects a collection", func() {
				collection, options := internal.IsCollection(*collectionDir)
				h.AssertTrue(t, collection)
				h.AssertEq(t, options, testCase.templates)
			})
		})
	}
}
