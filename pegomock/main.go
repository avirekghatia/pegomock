// Copyright 2016 Peter Goetz
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/petergtz/pegomock/pegomock/filehandling"
	"github.com/petergtz/pegomock/pegomock/remove"
	"github.com/petergtz/pegomock/pegomock/util"
	"github.com/petergtz/pegomock/pegomock/watch"
)

var (
	app = kingpin.New("pegomock", "Generates mocks based on interfaces.")
)

func main() {
	Run(os.Args, os.Stderr, os.Stdin, app, make(chan bool))
}

func Run(cliArgs []string, out io.Writer, in io.Reader, app *kingpin.Application, done chan bool) {

	workingDir, err := os.Getwd()
	app.FatalIfError(err, "")

	var (
		generateCmd    = app.Command("generate", "Generate mocks based on the args provided. ")
		destination    = generateCmd.Flag("output", "Output file; defaults to mock_<interface>_test.go.").Short('o').String()
		destinationDir = generateCmd.Flag("output-dir", "Output directory; defaults to current directory. If set, package name defaults to this directory, unless explicitly overridden.").String()
		packageOut     = generateCmd.Flag("package", "Package of the generated code; defaults to the package from which pegomock was executed suffixed with _test").String()
		// TODO: self_package was taken as is from GoMock.
		//       Still don't understand what it's really there for.
		//       So for now it's not tested.
		selfPackage            = generateCmd.Flag("self_package", "If set, the package this mock will be part of.").String()
		debugParser            = generateCmd.Flag("debug", "Print debug information.").Short('d').Bool()
		shouldGenerateMatchers = generateCmd.Flag("generate-matchers", "Generate matchers for all non built-in types in a \"matchers\" "+
			"directory in the same directory where the mock file gets generated.").Short('m').Default("false").Bool()
		matchersDestination = generateCmd.Flag("matchers-dir", "Generate matchers in the specified directory; defaults to "+
			filepath.Join("<mockdir>", "matchers")).Short('p').String()
		useExperimentalModelGen = generateCmd.Flag("use-experimental-model-gen", "pegomock includes a new experimental source parser based on "+
			"golang.org/x/tools/go/loader. It's currently experimental, but should be more powerful "+
			"than the current reflect-based modelgen. E.g. reflect cannot detect method parameter names,"+
			" and has to generate them based on a pattern. In a code editor with code assistence, this doesn't provide good help. "+
			"\n\nThis option only works when specifying package path + interface, not with .go source files. Also, you can only specify *one* interface. This option cannot be used with the watch command.").Bool()
		generateCmdArgs = generateCmd.Arg("args", "A (optional) Go package path + space-separated interface or a .go file").Required().Strings()

		watchCmd       = app.Command("watch", "Watch over changes in interfaces and regenerate mocks if changes are detected.")
		watchRecursive = watchCmd.Flag("recursive", "Recursively watch sub-directories as well.").Short('r').Bool()
		watchPackages  = watchCmd.Arg("directories...", "One or more directories of Go packages to watch").Strings()

		removeMocks          = app.Command("remove", "Remove mocks generated by Pegomock")
		removeRecursive      = removeMocks.Flag("recursive", "Remove recursively in all sub-directories").Default("false").Short('r').Bool()
		removeNonInteractive = removeMocks.Flag("non-interactive", "Don't ask for confirmation. Useful for scripts.").Default("false").Short('n').Bool()
		removeDryRun         = removeMocks.Flag("dry-run", "Just show what would be done. Don't delete anything.").Default("false").Short('d').Bool()
		removeSilent         = removeMocks.Flag("silent", "Don't write anything to standard out.").Default("false").Short('s').Bool()
		removePath           = removeMocks.Arg("path", "Use as root directory instead of current working directory.").Default("").String()
	)

	app.Writer(out)
	switch kingpin.MustParse(app.Parse(cliArgs[1:])) {

	case generateCmd.FullCommand():
		if err := util.ValidateArgs(*generateCmdArgs); err != nil {
			app.FatalUsage(err.Error())
		}
		sourceArgs, err := util.SourceArgs(*generateCmdArgs)
		if err != nil {
			app.FatalUsage(err.Error())
		}

		if *destination != "" && *destinationDir != "" {
			app.FatalUsage("Cannot use --output and --output-dir together")
		}

		realPackageOut := *packageOut
		if *packageOut == "" {
			realPackageOut = filepath.Base(workingDir) + "_test"
		}

		realDestination := *destination
		realDestinationDir := workingDir
		if *destinationDir != "" {
			realDestinationDir, err = filepath.Abs(*destinationDir)
			app.FatalIfError(err, "")
			if *packageOut == "" {
				realPackageOut = filepath.Base(*destinationDir)
			}
			if util.SourceMode(sourceArgs) {
				realDestination = filepath.Join(*destinationDir, "mock_"+strings.TrimSuffix(sourceArgs[0], ".go")+".go")
			} else {
				realDestination = filepath.Join(*destinationDir, "mock_"+strings.ToLower(sourceArgs[len(sourceArgs)-1])+".go")
			}
		}

		filehandling.GenerateMockFileInOutputDir(
			sourceArgs,
			realDestinationDir,
			realDestination,
			realPackageOut,
			*selfPackage,
			*debugParser,
			out,
			*useExperimentalModelGen,
			*shouldGenerateMatchers,
			*matchersDestination)

	case watchCmd.FullCommand():
		var targetPaths []string
		if len(*watchPackages) == 0 {
			targetPaths = []string{workingDir}
		} else {
			targetPaths = *watchPackages
		}
		watch.CreateWellKnownInterfaceListFilesIfNecessary(targetPaths)
		util.Ticker(watch.NewMockFileUpdater(targetPaths, *watchRecursive).Update, 2*time.Second, done)

	case removeMocks.FullCommand():
		path := *removePath
		if path == "" {
			var e error
			path, e = os.Getwd()
			app.FatalIfError(e, "Could not get current working directory")
		}
		remove.Remove(path, *removeRecursive, !*removeNonInteractive, *removeDryRun, *removeSilent, out, in, os.Remove)
	}
}
