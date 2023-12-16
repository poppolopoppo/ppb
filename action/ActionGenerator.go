package action

import (
	internal_io "github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
)

func BuildAction(action Action, staticDeps ...utils.BuildAlias) utils.BuildFactoryTyped[Action] {
	return utils.WrapBuildFactory(func(bi utils.BuildInitializer) (Action, error) {
		rules := action.GetAction()

		// track client dependencies
		if err := bi.DependsOn(staticDeps...); err != nil {
			return nil, err
		}

		// track executable file
		if err := bi.NeedFile(rules.Executable); err != nil {
			return nil, err
		}

		// track dependent actions as build dependency and as a member alias list
		if err := bi.DependsOn(rules.Dependencies...); err != nil {
			return nil, err
		}

		// create output directories
		outputDirs := utils.DirSet{}
		for _, filename := range rules.Outputs {
			outputDirs.AppendUniq(filename.Dirname)
		}
		for _, filename := range rules.Extras {
			outputDirs.AppendUniq(filename.Dirname)
		}

		for _, directory := range outputDirs {
			if _, err := internal_io.BuildDirectoryCreator(directory).Need(bi); err != nil {
				return nil, err
			}
		}

		return action, nil
	})
}
