package daemon

import (
	"fmt"
	"path/filepath"

	fpcmd "github.com/Manta-Network/manta-fp/bbn-fp/cmd"
	fpcfg "github.com/Manta-Network/manta-fp/bbn-fp/config"
	"github.com/Manta-Network/manta-fp/util"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/jessevdk/go-flags"
	"github.com/spf13/cobra"
)

// CommandInit returns the init command of bfpd daemon that starts the config dir.
func CommandInit() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "init",
		Short:   "Initialize a bbn-fp home directory.",
		Long:    `Creates a new bbn-fp home directory with default config`,
		Example: `bfpd init --home /home/user/.bfpd --force`,
		Args:    cobra.NoArgs,
		RunE:    fpcmd.RunEWithClientCtx(runInitCmd),
	}
	cmd.Flags().Bool(forceFlag, false, "Override existing configuration")
	return cmd
}

func runInitCmd(ctx client.Context, cmd *cobra.Command, _ []string) error {
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return err
	}

	homePath = util.CleanAndExpandPath(homePath)
	force, err := cmd.Flags().GetBool(forceFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", forceFlag, err)
	}

	if util.FileExists(homePath) && !force {
		return fmt.Errorf("home path %s already exists", homePath)
	}

	if err := util.MakeDirectory(homePath); err != nil {
		return err
	}
	// Create log directory
	logDir := fpcfg.LogDir(homePath)
	if err := util.MakeDirectory(logDir); err != nil {
		return err
	}

	defaultConfig := fpcfg.DefaultConfigWithHome(homePath)
	fileParser := flags.NewParser(&defaultConfig, flags.Default)

	return flags.NewIniParser(fileParser).WriteFile(fpcfg.CfgFile(homePath), flags.IniIncludeComments|flags.IniIncludeDefaults)
}
