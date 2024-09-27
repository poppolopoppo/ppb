package cmd

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var CommandVcxproj = NewCommand(
	"Configure",
	"vcxproj",
	"generate projects and solution for Visual Studio",
	OptionCommandRun(func(cc CommandContext) error {
		bg := utils.CommandEnv.BuildGraph().OpenWritePort(base.ThreadPoolDebugId{Category: "Vcxproj"})
		defer bg.Close()

		solutionFile := UFS.Output.File(CommandEnv.Prefix() + ".sln")

		result := NeedSlnSolutionBuilder(solutionFile).Build(bg)

		return result.Failure()
	}))

/***************************************
 * SlnSolutionBuilder
 ***************************************/

type SlnSolutionBuilder struct {
	ModuleAliases compile.ModuleAliases
	SlnSolution
}

func NeedSlnSolutionBuilder(outputFile Filename) BuildFactoryTyped[*SlnSolutionBuilder] {
	base.Assert(func() bool { return outputFile.Valid() })
	return MakeBuildFactory(func(init BuildInitializer) (SlnSolutionBuilder, error) {
		return SlnSolutionBuilder{
			SlnSolution: SlnSolution{
				SolutionOutput: outputFile.Normalize(),
			},
		}, nil
	})
}

func (x *SlnSolutionBuilder) Alias() BuildAlias {
	return MakeBuildAlias("SlnSolution", x.SolutionOutput.String())
}
func (x *SlnSolutionBuilder) Serialize(ar base.Archive) {
	base.SerializeSlice(ar, x.ModuleAliases.Ref())
	ar.Serializable(&x.SlnSolution)
}
func (x *SlnSolutionBuilder) Build(bc BuildContext) error {
	{ // reset SlnSolution, but keep path to output file
		outputFile := x.SlnSolution.SolutionOutput
		x.SlnSolution = SlnSolution{}
		x.SlnSolution.SolutionOutput = outputFile
		x.ModuleAliases = compile.ModuleAliases{}
	}

	base.LogClaim(LogCommand, "generating Microsoft Visual Studio SLN solution in '%v'", x.SolutionOutput)

	x.VisualStudioVersion = "16" // #TODO: not hard-coding visual studio version
	x.MinimumVisualStudioVersion = SlnDefaultMinimumVisualStudioVersion

	// collect every module in solution
	modules, err := compile.NeedAllBuildModules(bc)
	if err != nil {
		return err
	}
	x.ModuleAliases = base.Map(func(m compile.Module) compile.ModuleAlias {
		return m.GetModule().ModuleAlias
	}, modules...)
	sort.Slice(x.ModuleAliases, func(i, j int) bool {
		return x.ModuleAliases[i].String() < x.ModuleAliases[j].String()
	})

	// collect every solution config
	x.Configs = []SlnSolutionConfig{}
	if err := compile.ForeachEnvironmentAlias(func(ea compile.EnvironmentAlias) error {
		config := SlnSolutionConfig{
			Platform: x.SolutionPlatform(ea.PlatformName),
			Config:   ea.ConfigName,
		}
		config.SolutionPlatform = config.Platform
		config.SolutionConfig = config.Config
		x.Configs = append(x.Configs, config)
		return nil
	}); err != nil {
		return err
	}
	sort.Slice(x.Configs, func(i, j int) bool {
		if cmp := strings.Compare(x.Configs[i].Platform, x.Configs[j].Platform); cmp == 0 {
			return x.Configs[i].Config < x.Configs[j].Config
		} else {
			return cmp < 0
		}
	})

	solutionFolders := make(map[string]*base.StringSet)

	// collect every generated projects
	projects := make([]*VcxProject, len(x.ModuleAliases))
	for i, moduleAlias := range x.ModuleAliases {
		project, err := NeedVcxProjectBuilder(moduleAlias).Need(bc)
		if err != nil {
			return err
		}

		projects[i] = &project.VcxProject

		projectAbsolutePath := project.ProjectOutput.String()
		x.Projects.Append(projectAbsolutePath)

		if project.ShouldBuild {
			x.BuildProjects.Append(projectAbsolutePath)
		}

		if len(project.SolutionFolder) > 0 {
			if folders, ok := solutionFolders[project.SolutionFolder]; ok {
				folders.AppendUniq(projectAbsolutePath)
			} else {
				newSet := base.NewStringSet(projectAbsolutePath)
				solutionFolders[project.SolutionFolder] = &newSet
			}
		}
	}

	// meta project for build inspection/regen/natvis
	if project, err := x.createBuildConfigProject(bc, &solutionFolders); err == nil {
		projects = append(projects, project)
	} else {
		return err
	}

	// sort everything to be deterministic
	x.BuildProjects.Sort()
	x.DeployProjects.Sort()
	x.Projects.Sort()

	// collect every solution folders
	x.Folders = make([]SlnSolutionFolder, 0, len(solutionFolders))
	for namespace, projects := range solutionFolders {
		projects.Sort()
		x.Folders = append(x.Folders, SlnSolutionFolder{
			Path:     namespace,
			Projects: *projects,
		})
	}
	sort.Slice(x.Folders, func(i, j int) bool {
		return x.Folders[i].Path < x.Folders[j].Path
	})

	// finally, generate sln solution file
	generator := NewSlnSolutionGenerator(&x.SlnSolution)

	if err := UFS.CreateBuffered(x.SolutionOutput, func(w io.Writer) error {
		return generator.GenerateSLN(
			base.NewStructuredFile(w, "\t", false),
			projects...,
		)
	}, base.TransientPage4KiB); err != nil {
		return err
	}

	return bc.OutputFile(x.SolutionOutput)
}

func (x *SlnSolutionBuilder) createBuildConfigProject(bc BuildContext, solutionFolders *map[string]*base.StringSet) (*VcxProject, error) {
	buildProject := VcxProject{}
	buildProject.ProjectOutput = UFS.Projects.File("BuildConfig.vcxproj")
	buildProject.ProjectGuid = base.StringFingerprint(buildProject.ProjectOutput.String()).Guid()
	buildProject.SolutionFolder = "Build"
	buildProject.BasePath = UFS.Root

	// #TODO: remove PPE files from this "build" project
	buildProject.Files.Append(
		UFS.Root.File(CommandEnv.Prefix()).ReplaceExt(".go"),
		UFS.Root.File("README.md"),
		UFS.Root.File("TODO.md"),
		UFS.Root.File(".gitignore"),
		UFS.Source.File("cpp.hint"),
		UFS.Source.File(".gitignore"),
		UFS.Source.File("winnt_version.h"),
		UFS.Extras.Folder("Debug").File("PPE.natvis"),
		UFS.Extras.Folder("Debug").File("PPE.natstepfilter"),
	)
	buildProject.Files = base.RemoveUnless[Filename](func(f Filename) bool {
		return f.Exists()
	}, buildProject.Files...)

	source := compile.ModuleSource{}
	source.SourceDirs.Append(UFS.Root.Folder("Build"))
	source.SourceGlobs = base.NewStringSet("*.go", "*.bff", "*.json", "*.exe", "*.dll")
	source.ExcludedGlobs = base.NewStringSet(`*/.vs/*`, `*/.vscode/*`)
	if sourceFiles, err := source.GetFileSet(bc); err == nil {
		buildProject.Files.AppendUniq(sourceFiles...)
	} else {
		return nil, err
	}

	selfExecutable := fmt.Sprintf("%q -Ide -RootDir=%q", UFS.Executable, UFS.Root)
	buildProject.BuildCommand = fmt.Sprint(selfExecutable, " configure -and vcxroj")
	buildProject.RebuildCommand = fmt.Sprint(selfExecutable, " configure -and vcxroj -f")
	buildProject.RebuildCommand = fmt.Sprint(selfExecutable, " configure -and vcxroj -F")

	if err := compile.ForeachEnvironmentAlias(func(ea compile.EnvironmentAlias) error {
		buildProject.Configs = append(buildProject.Configs, VcxProjectConfig{
			Platform: x.SolutionPlatform(ea.PlatformName),
			Config:   ea.ConfigName,
		})
		return nil
	}); err != nil {
		return nil, err
	}

	x.Projects.Append(buildProject.ProjectOutput.String())
	(*solutionFolders)[buildProject.SolutionFolder] = &base.StringSet{buildProject.ProjectOutput.String()}

	generator := NewVcxProjectGenerator(&buildProject)
	if err := generator.GenerateAll(); err != nil {
		return nil, err
	}

	return &buildProject, bc.OutputFile(generator.ProjectOutput, generator.FiltersOutput)
}

/***************************************
 * VcxProjectBuilder
 ***************************************/

type VcxProjectBuilder struct {
	ModuleAlias compile.ModuleAlias
	VcxProject
	VisualStudioCanonicalPath
}

func NeedVcxProjectBuilder(moduleAlias compile.ModuleAlias) BuildFactoryTyped[*VcxProjectBuilder] {
	return MakeBuildFactory(func(bi BuildInitializer) (VcxProjectBuilder, error) {
		return VcxProjectBuilder{
			ModuleAlias: moduleAlias,
		}, bi.DependsOn(moduleAlias.Alias())
	})
}

func (x *VcxProjectBuilder) Alias() BuildAlias {
	return MakeBuildAlias("VcxProject", x.ModuleAlias.String())
}
func (x *VcxProjectBuilder) Serialize(ar base.Archive) {
	ar.Serializable(&x.ModuleAlias)
	ar.Serializable(&x.VcxProject)
}
func (x *VcxProjectBuilder) Build(bc BuildContext) error {
	x.VcxProject = VcxProject{}

	// retrieve associated module (1 project == 1 module)
	module, err := compile.FindBuildModule(bc, x.ModuleAlias)
	if err != nil {
		return err
	}

	moduleRules := module.GetModule()

	// retrieve generated units (1 project == n config|plaform configs)
	var units []*compile.Unit
	if err = compile.ForeachEnvironmentAlias(func(ea compile.EnvironmentAlias) error {
		unit, err := compile.FindBuildUnit(bc, compile.TargetAlias{
			ModuleAlias:      moduleRules.ModuleAlias,
			EnvironmentAlias: ea,
		})
		if err == nil {
			if err = bc.DependsOn(unit.Alias()); err == nil {
				units = append(units, unit)
			}
		}
		return err
	}); err != nil {
		return err
	}

	relativePath := moduleRules.ModuleDir.Relative(UFS.Source)

	x.ProjectGuid = base.StringFingerprint(x.ModuleAlias.String()).Guid()
	x.BasePath = moduleRules.ModuleDir
	x.ProjectOutput = UFS.Projects.AbsoluteFile(relativePath).ReplaceExt(".vcxproj")
	x.SolutionFolder = x.ModuleAlias.NamespaceName
	x.ShouldBuild = false

	// base.LogClaim(LogCommand, "generating Microsoft Visual Studio VCX project in '%v'", x.ProjectOutput)

	allowedFileExtensions := base.NewStringSet(`*.h`, `*.rc`, `*.natvis`, `*.natstepfilter`, `*.editorconfig`, `.gitconfig`)
	patternsToExclude := base.NewStringSet(`*/.vs/*`, `*/.vscode/*`)

	// allow debugging PS4
	x.ProjectImports = append(x.ProjectImports, VcxProjectImport{
		Condition: "'$(ConfigurationType)' == 'Makefile' and Exists('$(VCTargetsPath)\\Platforms\\$(Platform)\\SCE.Makefile.$(Platform).targets')",
		Project:   "$(VCTargetsPath)\\Platforms\\$(Platform)\\SCE.Makefile.$(Platform).targets",
	})

	// alow debugging Android
	x.ProjectImports = append(x.ProjectImports, VcxProjectImport{
		Condition: "'$(ConfigurationType)' == 'Makefile' and '$(AndroidAPILevel)' != '' and Exists('$(VCTargetsPath)\\Application Type\\$(ApplicationType)\\$(ApplicationTypeRevision)\\Android.Common.targets')",
		Project:   "$(VCTargetsPath)\\Application Type\\$(ApplicationType)\\$(ApplicationTypeRevision)\\Android.Common.targets",
	})

	// parse every project config from build units
	x.Configs = make([]VcxProjectConfig, len(units))
	for i, u := range units {
		x.ShouldBuild = x.ShouldBuild || (u.Payload != compile.PAYLOAD_HEADERS)

		if err := x.vcxProjectConfig(&x.Configs[i], u); err != nil {
			return err
		}

		source := u.Source
		source.SourceGlobs.AppendUniq(allowedFileExtensions...)
		source.SourceDirs.AppendUniq(u.Source.SourceDirs...)
		source.SourceDirs.AppendUniq(u.Source.ExtraDirs...)
		source.ExcludedGlobs.AppendUniq(patternsToExclude...)
		source.SourceFiles.AppendUniq(u.Source.ExtraFiles...)

		if publicDir := u.ModuleDir.Folder("Public"); publicDir.Exists() {
			source.SourceDirs.AppendUniq(publicDir)
		}

		if u.PCH != compile.PCH_DISABLED {
			source.SourceFiles.AppendUniq(u.PrecompiledHeader, u.PrecompiledSource)
		}

		if gitignore := u.ModuleDir.File(".gitignore"); gitignore.Exists() {
			source.SourceFiles.AppendUniq(gitignore)
		}

		sourceFiles, err := source.GetFileSet(bc)
		if err != nil {
			return err
		}

		x.Files.AppendUniq(sourceFiles...)
	}

	// sort everything so we are deterministic
	x.Files.Sort()
	x.AssemblyReferences.Sort()
	x.ProjectReferences.Sort()
	x.SourceControlBindings.Sort()

	sort.Slice(x.Configs, func(i, j int) bool {
		if cmp := strings.Compare(x.Configs[i].Platform, x.Configs[j].Platform); cmp == 0 {
			return x.Configs[i].Config < x.Configs[j].Config
		} else {
			return cmp < 0
		}
	})
	sort.Slice(x.FileTypes, func(i, j int) bool {
		return x.FileTypes[i].FileType < x.FileTypes[j].FileType
	})
	sort.Slice(x.ProjectImports, func(i, j int) bool {
		return x.ProjectImports[i].Project < x.ProjectImports[j].Project
	})

	// finally, generate vcxproject file
	generator := NewVcxProjectGenerator(&x.VcxProject)
	if err = generator.GenerateAll(); err != nil {
		return err
	}

	return bc.OutputFile(generator.ProjectOutput, generator.FiltersOutput)
}

func (x *VcxProjectBuilder) vcxProjectConfig(config *VcxProjectConfig, u *compile.Unit) error {
	config.Platform = x.SolutionPlatform(u.TargetAlias.PlatformName)
	config.Config = u.TargetAlias.ConfigName
	config.PlatformToolset = fmt.Sprint("v", u.Facet.Exports.Get("VisualStudio/PlatformToolset"))
	config.OutputFile = u.OutputFile
	config.OutputDirectory = u.OutputFile.Dirname
	config.IntermediateDirectory = u.IntermediateDir
	config.BuildLogFile = u.IntermediateDir.File("BuildLog.log")
	config.AdditionalOptions = u.AnalysisOptions.Join(" ")
	config.PreprocessorDefinitions = u.Defines.Join(";")
	config.ForcedIncludes = u.ForceIncludes

	config.IncludeSearchPath = NewDirSet(u.IncludePaths...)
	config.IncludeSearchPath.Append(u.ExternIncludePaths...)
	config.IncludeSearchPath.Append(u.SystemIncludePaths...)

	if u.Payload.HasOutput() {
		target := u.TargetAlias.String()

		selfExecutable := fmt.Sprintf("%q -Ide -RootDir=%q ", UFS.Executable, UFS.Root)
		config.BuildCommand = fmt.Sprint(selfExecutable, " build -- ", target)
		config.RebuildCommand = fmt.Sprint(selfExecutable, " build -Rebuild -- ", target)
		config.CleanCommand = fmt.Sprint(selfExecutable, " build -Clean -- ", target)

		if u.Payload == compile.PAYLOAD_EXECUTABLE {
			config.LocalDebuggerCommand = u.OutputFile
			config.LocalDebuggerWorkingDirectory = u.OutputFile.Dirname

			const htmlLineFeed = `&#10;`
			config.LocalDebuggerEnvironment = strings.Join(append(u.Environment.Export(), "^$(LocalDebuggerEnvironment)"), htmlLineFeed)
		}
	}
	return nil
}

/***************************************
 * Native SLN solution generation
 ***************************************/

type SlnAdditionalOptions struct {
	BuildProjects  base.StringSet
	DeployProjects base.StringSet
}

func (x *SlnAdditionalOptions) Serialize(ar base.Archive) {
	ar.Serializable(&x.BuildProjects)
	ar.Serializable(&x.DeployProjects)
}

type SlnSolutionConfig struct {
	Platform string
	Config   string

	SolutionPlatform string
	SolutionConfig   string

	SlnAdditionalOptions
}

func (x *SlnSolutionConfig) Serialize(ar base.Archive) {
	ar.String(&x.Platform)
	ar.String(&x.Config)
	ar.String(&x.SolutionPlatform)
	ar.String(&x.SolutionConfig)
	ar.Serializable(&x.SlnAdditionalOptions)
}

type SlnSolutionFolder struct {
	Path     string
	Projects base.StringSet
	Items    FileSet
}

func (x *SlnSolutionFolder) Serialize(ar base.Archive) {
	ar.String(&x.Path)
	ar.Serializable(&x.Projects)
	ar.Serializable(&x.Items)
}

type SlnSolutionDependencies struct {
	Projects     base.StringSet
	Dependencies base.StringSet
}

func (x *SlnSolutionDependencies) Serialize(ar base.Archive) {
	ar.Serializable(&x.Projects)
	ar.Serializable(&x.Dependencies)
}

type SlnSolution struct {
	SolutionOutput Filename

	Projects     base.StringSet
	Configs      []SlnSolutionConfig
	Folders      []SlnSolutionFolder
	Dependencies []SlnSolutionDependencies

	VisualStudioVersion        string
	MinimumVisualStudioVersion string

	SlnAdditionalOptions
	VisualStudioCanonicalPath
}

func (x *SlnSolution) Serialize(ar base.Archive) {
	ar.Serializable(&x.SolutionOutput)
	ar.Serializable(&x.Projects)
	base.SerializeSlice(ar, &x.Configs)
	base.SerializeSlice(ar, &x.Folders)
	base.SerializeSlice(ar, &x.Dependencies)
	ar.String(&x.VisualStudioVersion)
	ar.String(&x.MinimumVisualStudioVersion)
	ar.Serializable(&x.SlnAdditionalOptions)
	ar.Serializable(&x.SlnAdditionalOptions)
}

type SlnSolutionGenerator struct {
	*SlnSolution
}

const SlnDefaultVisualStudioVersion = "14.0.22823.1"        // Visual Studio 2015 RC
const SlnDefaultMinimumVisualStudioVersion = "10.0.40219.1" // Visual Studio Express 2010

func NewSlnSolutionGenerator(sln *SlnSolution) (result SlnSolutionGenerator) {
	result.SlnSolution = sln
	return
}

func (x *SlnSolutionGenerator) GenerateSLN(sln *base.StructuredFile, projects ...*VcxProject) error {
	solutionBasePath := x.SolutionOutput.Dirname

	folderPaths := make(base.StringSet, 0, len(x.Folders))
	projectsToFolders := make(base.StringSet, 0, len(x.Projects))

	// headers
	shortVersion := x.VisualStudioVersion
	if index := strings.IndexRune(shortVersion, '.'); index >= 0 {
		shortVersion = shortVersion[:index]
	}
	shortVersionInt, err := strconv.Atoi(shortVersion)
	if err != nil {
		return err
	}
	sln.Println("Microsoft Visual Studio Solution File, Format Version 12.00")
	if shortVersionInt >= 16 {
		sln.Println("# Visual Studio Version %s", shortVersion)
	} else {
		sln.Println("# Visual Studio %s", shortVersion)
	}
	sln.Println("VisualStudioVersion = %s", x.VisualStudioVersion)
	sln.Println("MinimumVisualStudioVersion = %s", x.MinimumVisualStudioVersion)

	// project listings
	for _, project := range projects {
		projectName := project.ProjectOutput.TrimExt()
		projectAbsolutePath := project.ProjectOutput.String()
		solutionRelativePath := x.CanonicalizeFile(solutionBasePath, project.ProjectOutput)

		projectGuid := strings.ToUpper(project.ProjectGuid)
		projectTypeGuid := strings.ToUpper(project.GetProjectTypeGuid())

		sln.Println("Project(%q) = \"%s\", \"%s\", \"%s\"",
			projectTypeGuid, projectName, solutionRelativePath, projectGuid)
		sln.BeginIndent()

		// dependencies
		dependencyGuids := make(base.StringSet, 0, 64)
		for _, deps := range x.Dependencies {
			if !deps.Projects.Contains(projectAbsolutePath) {
				continue
			}

			for _, dependency := range deps.Dependencies {
				for _, dependencyProject := range projects {
					if dependencyProject.ProjectOutput.String() == dependency {
						dependencyGuids.Append(dependencyProject.ProjectGuid)
					}
				}
			}
		}

		if len(dependencyGuids) > 0 {
			sln.Println("ProjectSection(ProjectDependencies) = postProject")
			sln.ScopeIndent(func() {
				for _, guid := range dependencyGuids {
					sln.Println("%s = %s", guid, guid)
				}
			})
			sln.Println("EndProjectSection")
		}

		sln.EndIndent()
		sln.Println("EndProject")

		// check if project is in solution folder
		for _, folder := range x.Folders {
			if folder.Projects.Contains(projectAbsolutePath) {
				solutionFolderGuid := base.StringFingerprint(folder.Path).Guid()
				solutionFolderGuid = strings.ToUpper(solutionFolderGuid)

				projectsToFolders = append(projectsToFolders,
					fmt.Sprintf("%s = %s", projectGuid, solutionFolderGuid))
			}
		}
	}

	// create every intermediate solution folder and sort them
	for _, folder := range x.Folders {
		folderPaths.AppendUniq(folder.Path)

		for path := folder.Path; ; {
			if path = MakeParentFolder(path); len(path) > 0 {
				folderPaths.AppendUniq(path)
			} else {
				break
			}
		}
	}
	folderPaths.Sort()

	// solution folders listings
	for _, folderPath := range folderPaths {
		solutionFolderName := MakeBasename(folderPath)

		solutionFolderGuid := base.StringFingerprint(folderPath).Guid()
		solutionFolderGuid = strings.ToUpper(solutionFolderGuid)

		sln.Println("Project(\"{2150E333-8FDC-42A3-9474-1A3956D46DE8}\") = \"%s\", \"%s\", \"%s\"",
			solutionFolderName, solutionFolderName, solutionFolderGuid)
		sln.BeginIndent()

		for _, solutionFolder := range x.Folders {
			if solutionFolder.Path == folderPath {
				if solutionFolder.Items.Len() == 0 {
					continue
				}

				sortedItems := NewFileSet(solutionFolder.Items...)
				sortedItems.Sort() // Visual Studio will invalidate to sort the items if we don't do this

				sln.Println("ProjectSection(SolutionItems) = preProject")
				sln.ScopeIndent(func() {
					for _, item := range sortedItems {
						relativePath := x.CanonicalizeFile(solutionBasePath, item)
						sln.Println("%s = %s", relativePath, relativePath)
					}
				})
				sln.Println("EndProjectSection")
			}
		}

		sln.EndIndent()
		sln.Println("EndProject")
	}

	// global
	sln.Println("Global")
	sln.BeginIndent()

	// solution configuration platforms
	sln.Println("GlobalSection(SolutionConfigurationPlatforms) = preSolution")
	sln.BeginIndent()

	for _, it := range x.Configs {
		sln.Println("%s|%s = %s|%s",
			it.SolutionConfig, it.SolutionPlatform,
			it.SolutionConfig, it.SolutionPlatform)
	}

	sln.EndIndent()
	sln.Println("EndGlobalSection")

	// project configuration platforms
	sln.Println("GlobalSection(ProjectConfigurationPlatforms) = postSolution")
	sln.BeginIndent()

	for _, project := range projects {
		projectGuid := strings.ToUpper(project.ProjectGuid)
		projectAbsolutePath := project.ProjectOutput.String()

		for _, config := range x.Configs {
			sln.Println("%s.%s|%s.ActiveCfg = %s|%s",
				projectGuid,
				config.SolutionConfig, config.SolutionPlatform,
				config.Config, config.Platform)

			if projectIsActive := x.BuildProjects.Contains(projectAbsolutePath) || config.BuildProjects.Contains(projectAbsolutePath); projectIsActive {
				sln.Println("%s.%s|%s.Build.0 = %s|%s",
					projectGuid,
					config.SolutionConfig, config.SolutionPlatform,
					config.Config, config.Platform)
			}

			if projectIsDeployed := config.DeployProjects.Contains(projectAbsolutePath); projectIsDeployed {
				sln.Println("%s.%s|%s.Deploy.0 = %s|%s",
					projectGuid,
					config.SolutionConfig, config.SolutionPlatform,
					config.Config, config.Platform)
			}

		}
	}

	sln.EndIndent()
	sln.Println("EndGlobalSection")

	sln.Println("GlobalSection(SolutionProperties) = preSolution")
	sln.ScopeIndent(func() {
		sln.Println("HideSolutionNode = FALSE")
	})
	sln.Println("EndGlobalSection")

	// nested projects
	if projectsToFolders.Len() > 0 || folderPaths.Len() > 0 {
		sln.Println("GlobalSection(NestedProjects) = preSolution")
		sln.BeginIndent()

		// write projects to folders relationships
		for _, projectToFolder := range projectsToFolders {
			sln.Println(projectToFolder)
		}

		// write every intermediate path
		for _, solutionFolder := range folderPaths {
			folderParent := MakeParentFolder(solutionFolder)
			if len(folderParent) == 0 {
				continue
			}

			parentGuid := base.StringFingerprint(folderParent).Guid()
			parentGuid = strings.ToLower(parentGuid)

			folderGuid := base.StringFingerprint(solutionFolder).Guid()
			folderGuid = strings.ToLower(folderGuid)

			sln.Println("%s = %s", folderGuid, parentGuid)
		}

		sln.EndIndent()
		sln.Println("EndGlobalSection")
	}

	// footer
	sln.EndIndent()
	sln.Println("EndGlobal")

	return nil
}

/***************************************
 * Native VCXProj project generation
 ***************************************/

type VcxFileType struct {
	FileType string
	Pattern  string
}

func (x *VcxFileType) Serialize(ar base.Archive) {
	ar.String(&x.FileType)
	ar.String(&x.Pattern)
}

type VcxAdditionalOptions struct {
	// Compilation (optional)
	BuildCommand   string
	RebuildCommand string
	CleanCommand   string

	// Compilation Input/Output (optional)
	OutputFile            Filename
	OutputDirectory       Directory
	IntermediateDirectory Directory
	BuildLogFile          Filename

	// Intellisense Options (optional)
	PreprocessorDefinitions string
	IncludeSearchPath       DirSet
	ForcedIncludes          FileSet
	AssemblySearchPath      DirSet
	ForcedUsingAssemblies   base.StringSet
	AdditionalOptions       string

	// Debugging Options (optional)
	LocalDebuggerCommand          Filename
	LocalDebuggerCommandArguments string
	LocalDebuggerWorkingDirectory Directory
	LocalDebuggerEnvironment      string

	RemoteDebuggerCommand          Filename
	RemoteDebuggerCommandArguments string
	RemoteDebuggerWorkingDirectory Directory

	DebuggerFlavor              string
	ApplicationType             string
	ApplicationTypeRevision     string
	TargetLinuxPlatform         string
	LinuxProjectType            string
	PackagePath                 Directory
	AdditionalSymbolSearchPaths DirSet
	AndroidApkLocation          Directory

	// Misc
	PlatformToolset string
	RootNamespace   string
	Keyword         string
}

func (x *VcxAdditionalOptions) Serialize(ar base.Archive) {
	ar.String(&x.BuildCommand)
	ar.String(&x.RebuildCommand)
	ar.String(&x.CleanCommand)
	ar.Serializable(&x.OutputFile)
	ar.Serializable(&x.OutputDirectory)
	ar.Serializable(&x.IntermediateDirectory)
	ar.Serializable(&x.BuildLogFile)
	ar.String(&x.PreprocessorDefinitions)
	ar.Serializable(&x.IncludeSearchPath)
	ar.Serializable(&x.ForcedIncludes)
	ar.Serializable(&x.AssemblySearchPath)
	ar.Serializable(&x.ForcedUsingAssemblies)
	ar.String(&x.AdditionalOptions)
	ar.Serializable(&x.LocalDebuggerCommand)
	ar.String(&x.LocalDebuggerCommandArguments)
	ar.Serializable(&x.LocalDebuggerWorkingDirectory)
	ar.String(&x.LocalDebuggerEnvironment)
	ar.Serializable(&x.RemoteDebuggerCommand)
	ar.String(&x.RemoteDebuggerCommandArguments)
	ar.Serializable(&x.RemoteDebuggerWorkingDirectory)
	ar.String(&x.DebuggerFlavor)
	ar.String(&x.ApplicationType)
	ar.String(&x.ApplicationTypeRevision)
	ar.String(&x.TargetLinuxPlatform)
	ar.String(&x.LinuxProjectType)
	ar.Serializable(&x.PackagePath)
	ar.Serializable(&x.AdditionalSymbolSearchPaths)
	ar.Serializable(&x.AndroidApkLocation)
	ar.String(&x.PlatformToolset)
	ar.String(&x.RootNamespace)
	ar.String(&x.Keyword)
}

type VcxProjectConfig struct {
	Platform string
	Config   string

	VcxAdditionalOptions
}

func (x *VcxProjectConfig) Serialize(ar base.Archive) {
	ar.String(&x.Platform)
	ar.String(&x.Config)
	ar.Serializable(&x.VcxAdditionalOptions)
}

type VcxProjectImport struct {
	Condition string
	Project   string
}

func (x *VcxProjectImport) Serialize(ar base.Archive) {
	ar.String(&x.Condition)
	ar.String(&x.Project)
}

type VcxProject struct {
	// Output Options
	ProjectOutput Filename

	// Input Options
	Files     FileSet
	BasePath  Directory
	FileTypes []VcxFileType

	// Build Config Options
	Configs []VcxProjectConfig

	// Reference Options
	AssemblyReferences base.StringSet
	ProjectReferences  base.StringSet

	// Project Import Options
	ProjectImports []VcxProjectImport

	// Other Options
	ApplicationEnvironment string
	DefaultLanguage        string
	ProjectGuid            string
	SourceControlBindings  base.StringSet
	SolutionFolder         string
	ShouldBuild            bool

	VcxAdditionalOptions
}

func (x *VcxProject) GetProjectTypeGuid() string {
	return "{8BC9CEB8-8B4A-11D0-8D11-00A0C91BC942}"
}
func (x *VcxProject) Serialize(ar base.Archive) {
	// Output Options
	ar.Serializable(&x.ProjectOutput)
	ar.Serializable(&x.Files)
	ar.Serializable(&x.BasePath)
	base.SerializeSlice(ar, &x.FileTypes)
	base.SerializeSlice(ar, &x.Configs)
	ar.Serializable(&x.AssemblyReferences)
	ar.Serializable(&x.ProjectReferences)
	base.SerializeSlice(ar, &x.ProjectImports)
	ar.String(&x.ApplicationEnvironment)
	ar.String(&x.DefaultLanguage)
	ar.String(&x.ProjectGuid)
	ar.Serializable(&x.SourceControlBindings)
	ar.String(&x.SolutionFolder)
	ar.Bool(&x.ShouldBuild)
	ar.Serializable(&x.VcxAdditionalOptions)
}

type VcxProjectGenerator struct {
	*VcxProject

	FiltersOutput Filename
	VisualStudioCanonicalPath
}

func NewVcxProjectGenerator(vcxproject *VcxProject) (result VcxProjectGenerator) {
	result.VcxProject = vcxproject
	result.FiltersOutput = result.ProjectOutput.ReplaceExt(".vcxproj.filters")
	return
}

func (x *VcxProjectGenerator) GenerateAll() error {
	if err := UFS.CreateBuffered(x.ProjectOutput, func(w io.Writer) error {
		return x.GenerateVCXProj(internal_io.NewXmlFile(w, false))
	}, base.TransientPage4KiB); err != nil {
		return err
	}

	// write .vcxproj.filters
	if err := UFS.CreateBuffered(x.FiltersOutput, func(w io.Writer) error {
		return x.GenerateVCXProjFilters(internal_io.NewXmlFile(w, false))
	}, base.TransientPage4KiB); err != nil {
		return err
	}
	return nil
}

func (x *VcxProjectGenerator) GenerateVCXProj(xml *internal_io.XmlFile) error {
	base.AssertNotIn(x.ProjectGuid, "")

	projectBasePath := x.ProjectOutput.Dirname

	// header
	xml.Println("<?xml version=\"1.0\" encoding=\"utf-8\"?>")
	xml.Println("<Project DefaultTargets=\"Build\" ToolsVersion=\"15.0\" xmlns=\"http://schemas.microsoft.com/developer/msbuild/2003\">")
	xml.BeginIndent()

	// project configuration
	xml.Tag("ItemGroup", func() {
		for _, it := range x.Configs {
			xml.Tag("ProjectConfiguration", func() {
				xml.InnerString("Configuration", it.Config)
				xml.InnerString("Platform", it.Platform)
			}, internal_io.XmlAttr{Name: "Include", Value: fmt.Sprintf("%s|%s", it.Config, it.Platform)})
		}
	}, internal_io.XmlAttr{Name: "Label", Value: "ProjectConfigurations"})

	// files
	xml.Tag("ItemGroup", func() {
		for _, file := range x.Files {
			relative := x.CanonicalizeFile(projectBasePath, file)

			var fileType *VcxFileType
			for i, it := range x.FileTypes {
				if MakeGlobRegexp(it.Pattern).MatchString(relative) {
					fileType = &x.FileTypes[i]
				}
			}

			closure := func() {
				xml.InnerString("FileType", fileType.FileType)
			}
			xml.Tag("CustomBuild", base.Blend(nil, closure, fileType != nil), internal_io.XmlAttr{Name: "Include", Value: relative})
		}
	})

	// references
	xml.Tag("ItemGroup", func() {
		// project references
		for _, projectRef := range x.ProjectReferences {
			pipe := strings.IndexRune(projectRef, '|')
			closure := func() {
				xml.InnerString("Project", projectRef[pipe+1:])
			}
			xml.Tag("ProjectReference", base.Blend(nil, closure, pipe > 0), internal_io.XmlAttr{Name: "Include", Value: projectRef[:pipe]})
		}
		// assembly references
		for _, assemblyRef := range x.AssemblyReferences {
			xml.Tag("Reference", nil, internal_io.XmlAttr{Name: "Include", Value: assemblyRef})
		}
	})

	// globals
	xml.Tag("PropertyGroup", func() {
		xml.InnerString("RootNamespace", x.RootNamespace)
		xml.InnerString("ProjectGuid", x.ProjectGuid)
		xml.InnerString("DefaultLanguage", x.DefaultLanguage)
		xml.InnerString("Keyword", "MakeFileProj")

		if len(x.SourceControlBindings) > 0 {
			xml.InnerString("SccProjectName", "SAK")
			xml.InnerString("SccAuxPath", "SAK")
			xml.InnerString("SccLocalPath", "SAK")
			xml.InnerString("SccProvider", "SAK")
		}

		xml.InnerString("ApplicationEnvironment", x.ApplicationEnvironment)
	}, internal_io.XmlAttr{Name: "Label", Value: "Globals"})

	// per-config globals
	for _, config := range x.Configs {
		if len(config.Keyword) == 0 &&
			len(config.RootNamespace) == 0 &&
			len(config.ApplicationType) == 0 &&
			len(config.ApplicationTypeRevision) == 0 &&
			len(config.TargetLinuxPlatform) == 0 &&
			len(config.LinuxProjectType) == 0 {
			continue
		}

		xml.Tag("PropertyGroup", func() {
			xml.InnerString("Keyword", config.Keyword)
			xml.InnerString("RootNamespace", config.RootNamespace)
			xml.InnerString("ApplicationType", config.ApplicationType)
			xml.InnerString("ApplicationTypeRevision", config.ApplicationTypeRevision)
			xml.InnerString("TargetLinuxPlatform", config.TargetLinuxPlatform)
			xml.InnerString("LinuxProjectType", config.LinuxProjectType)
		}, internal_io.XmlAttr{Name: "Condition", Value: fmt.Sprintf("'$(Configuration)|$(Platform)'=='%s|%s'", config.Config, config.Platform)},
			internal_io.XmlAttr{Name: "Label", Value: "Globals"})
	}

	// defaut props
	xml.Println("<Import Project=\"$(VCTargetsPath)\\Microsoft.Cpp.Default.props\" />")

	// configurations
	for _, config := range x.Configs {
		xml.Tag("PropertyGroup", func() {
			xml.InnerString("ConfigurationType", "Makefile")
			xml.InnerString("UseDebugLibraries", "false")

			xml.InnerString("PlatformToolset", config.PlatformToolset)
			xml.InnerString("LocalDebuggerCommandArguments", config.LocalDebuggerCommandArguments)
			xml.InnerString("LocalDebuggerCommand", x.CanonicalizePath(config.LocalDebuggerCommand.String()))
			xml.InnerString("LocalDebuggerEnvironment", config.LocalDebuggerEnvironment)

		}, internal_io.XmlAttr{Name: "Condition", Value: fmt.Sprintf("'$(Configuration)|$(Platform)'=='%s|%s'", config.Config, config.Platform)},
			internal_io.XmlAttr{Name: "Label", Value: "Configuration"})
	}

	// imports
	xml.Tag("Import", nil, internal_io.XmlAttr{Name: "Project", Value: "$(VCTargetsPath)\\Microsoft.Cpp.props"})
	xml.Tag("ImportGroup", nil, internal_io.XmlAttr{Name: "Label", Value: "ExtensionSettings"})

	// property sheets
	for _, config := range x.Configs {
		xml.Tag("ImportGroup", func() {
			xml.Println("<Import Project=\"$(UserRootDir)\\Microsoft.Cpp.$(Platform).user.props\" Condition=\"exists('$(UserRootDir)\\Microsoft.Cpp.$(Platform).user.props')\" Label=\"LocalAppDataPlatform\" />")
		}, internal_io.XmlAttr{Name: "Condition", Value: fmt.Sprintf("'$(Configuration)|$(Platform)'=='%s|%s'", config.Config, config.Platform)},
			internal_io.XmlAttr{Name: "Label", Value: "PropertySheets"})
	}

	// user macros
	xml.Println("<PropertyGroup Label=\"UserMacros\" />")

	// property group
	for _, config := range x.Configs {
		xml.Tag("PropertyGroup", func() {
			if config.Keyword == "Linux" {
				xml.InnerString("BuildCommandLine", config.BuildCommand)
				xml.InnerString("ReBuildCommandLine", config.RebuildCommand)
				xml.InnerString("CleanCommandLine", config.CleanCommand)
			} else {
				xml.InnerString("NMakeBuildCommandLine", config.BuildCommand)
				xml.InnerString("NMakeReBuildCommandLine", config.RebuildCommand)
				xml.InnerString("NMakeCleanCommandLine", config.CleanCommand)
			}

			xml.InnerString("NMakeOutput", config.OutputFile.String())

			xml.InnerString("NMakePreprocessorDefinitions", config.PreprocessorDefinitions)
			xml.InnerString("NMakeIncludeSearchPath", x.CanonicalizeDirs(projectBasePath, config.IncludeSearchPath...))
			xml.InnerString("NMakeForcedIncludes", x.CanonicalizeFiles(projectBasePath, config.ForcedIncludes...))
			xml.InnerString("NMakeAssemblySearchPath", x.CanonicalizeDirs(projectBasePath, config.AssemblySearchPath...))
			xml.InnerString("NMakeForcedUsingAssemblies", strings.Join(config.ForcedUsingAssemblies, ";"))

			xml.InnerString("AdditionalOptions", config.AdditionalOptions)

			xml.InnerString("DebuggerFlavor", config.DebuggerFlavor)
			xml.InnerString("LocalDebuggerWorkingDirectory", x.CanonicalizePath(config.LocalDebuggerWorkingDirectory.String()))
			xml.InnerString("IntDir", x.CanonicalizePath(config.IntermediateDirectory.String()))
			xml.InnerString("OutDir", x.CanonicalizePath(config.OutputDirectory.String()))
			xml.InnerString("PackagePath", x.CanonicalizePath(config.PackagePath.String()))
			xml.InnerString("AdditionalSymbolSearchPaths", x.CanonicalizeDirs(projectBasePath, config.AdditionalSymbolSearchPaths...))

			xml.InnerString("RemoteDebuggerCommand", x.CanonicalizePath(config.RemoteDebuggerCommand.String()))
			xml.InnerString("RemoteDebuggerCommandArguments", config.RemoteDebuggerCommandArguments)
			xml.InnerString("RemoteDebuggerWorkingDirectory", x.CanonicalizePath(config.RemoteDebuggerWorkingDirectory.String()))

		}, internal_io.XmlAttr{Name: "Condition", Value: fmt.Sprintf("'$(Configuration)|$(Platform)'=='%s|%s'", config.Config, config.Platform)})
	}

	// item definitions
	for _, config := range x.Configs {
		xml.Tag("ItemDefinitionGroup", func() {
			xml.Tag("BuildLog", func() {
				if config.BuildLogFile.Valid() {
					xml.InnerString("Path", x.CanonicalizePath(config.BuildLogFile.String()))
				} else {
					xml.Println("<Path />")
				}
			})
		}, internal_io.XmlAttr{Name: "Condition", Value: fmt.Sprintf("'$(Configuration)|$(Platform)'=='%s|%s'", config.Config, config.Platform)})
	}

	// footer
	xml.Println("<Import Project=\"$(VCTargetsPath)\\Microsoft.Cpp.targets\" />")
	xml.Println("<ImportGroup Label=\"ExtensionTargets\"></ImportGroup>")

	for _, imp := range x.ProjectImports {
		xml.Tag("Import", nil,
			internal_io.XmlAttr{Name: "Condition", Value: imp.Condition},
			internal_io.XmlAttr{Name: "Project", Value: imp.Project})
	}

	xml.EndIndent()
	xml.Print("</Project>") // no carriage return
	return nil
}

func (x *VcxProjectGenerator) GenerateVCXProjFilters(xml *internal_io.XmlFile) error {
	projectBasePath := x.ProjectOutput.Dirname

	// header
	xml.Println("<?xml version=\"1.0\" encoding=\"utf-8\"?>")
	xml.Println("<Project ToolsVersion=\"4.0\" xmlns=\"http://schemas.microsoft.com/developer/msbuild/2003\">")
	xml.BeginIndent()

	var expandedFolders DirSet

	// files
	xml.Println("<ItemGroup>")
	xml.BeginIndent()

	for _, file := range x.Files {
		xml.Tag("CustomBuild", func() {
			folder := x.CanonicalizeDir(x.BasePath, file.Dirname)
			if len(folder) == 0 || folder == "." {
				return
			}

			xml.InnerString("Filter", folder)

			if !expandedFolders.Contains(file.Dirname) {
				expandedFolders.Append(file.Dirname)

				if x.BasePath.IsParentOf(file.Dirname) && x.BasePath != file.Dirname {
					for d := file.Dirname.Parent(); d != x.BasePath; d = d.Parent() {
						if !expandedFolders.Contains(d) {
							expandedFolders.Append(d)
						} else {
							break // already visited
						}
					}
				}
			}

		}, internal_io.XmlAttr{Name: "Include", Value: x.CanonicalizeFile(projectBasePath, file)})
	}

	xml.EndIndent()
	xml.Println("</ItemGroup>")

	// folders
	expandedFolders.Sort()

	for _, folder := range expandedFolders {
		xml.Tag("ItemGroup", func() {
			canon := x.CanonicalizeDir(x.BasePath, folder)
			xml.Tag("Filter", func() {
				xml.InnerString("UniqueIdentifier", base.StringFingerprint(canon).Guid())
			}, internal_io.XmlAttr{Name: "Include", Value: canon})
		})
	}

	// footer
	xml.EndIndent()
	xml.Print("</Project>") // no carriage return
	return nil
}

/***************************************
 * Path canonicalization for Visual Studio
 ***************************************/

type VisualStudioCanonicalPath struct{}

func (x VisualStudioCanonicalPath) SolutionPlatform(platformName string) string {
	switch platformName {
	case "Win32":
		return "Win32"
	case "Win64":
		return "x64"
	default:
		base.UnexpectedValue(platformName)
		return ""
	}
}
func (x VisualStudioCanonicalPath) CanonicalizePath(s string) string {
	return SanitizePath(s, '\\')
}
func (x VisualStudioCanonicalPath) CanonicalizeDir(basePath Directory, d Directory) string {
	if d.Valid() {
		return x.CanonicalizePath(d.Relative(basePath))
	}
	return ""
}
func (x VisualStudioCanonicalPath) CanonicalizeFile(basePath Directory, f Filename) string {
	if f.Valid() {
		return x.CanonicalizePath(f.Relative(basePath))
	}
	return ""
}
func (x VisualStudioCanonicalPath) CanonicalizeFiles(basePath Directory, files ...Filename) string {
	return base.JoinString(";", files...)
}
func (x VisualStudioCanonicalPath) CanonicalizeDirs(basePath Directory, dirs ...Directory) string {
	return base.JoinString(";", dirs...)
}
