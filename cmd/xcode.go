package cmd

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	log "github.com/Sirupsen/logrus"

	"github.com/bitrise-io/go-utils/cmdex"
	"github.com/bitrise-io/go-utils/colorstring"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/goinp/goinp"
	"github.com/bitrise-tools/codesigndoc/provprofile"
	"github.com/bitrise-tools/codesigndoc/xcode"
	"github.com/spf13/cobra"
)

// xcodeCmd represents the xcode command
var xcodeCmd = &cobra.Command{
	Use:   "xcode",
	Short: "Xcode project scanner",
	Long:  `Scan an Xcode project`,

	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          scanXcodeProject,
}

var (
	paramXcodeProjectFilePath = ""
	paramXcodeScheme          = ""
)

func init() {
	scanCmd.AddCommand(xcodeCmd)

	xcodeCmd.Flags().StringVar(&paramXcodeProjectFilePath,
		"file", "",
		"Xcode Project/Workspace file path")
	xcodeCmd.Flags().StringVar(&paramXcodeScheme,
		"scheme", "",
		"Xcode Scheme")
}

func printXcodeScanFinishedWithError(format string, args ...interface{}) error {
	return printFinishedWithError("Xcode", format, args...)
}

func scanXcodeProject(cmd *cobra.Command, args []string) error {
	absExportOutputDirPath, err := initExportOutputDir()
	if err != nil {
		return printXcodeScanFinishedWithError("Failed to prepare Export directory: %s", err)
	}

	projectPath := paramXcodeProjectFilePath
	if projectPath == "" {
		askText := `Please drag-and-drop your Xcode Project (` + colorstring.Green(".xcodeproj") + `)
   or Workspace (` + colorstring.Green(".xcworkspace") + `) file, the one you usually open in Xcode,
   then hit Enter.

  (Note: if you have a Workspace file you should most likely use that)`
		fmt.Println()
		projpth, err := goinp.AskForPath(askText)
		if err != nil {
			return printXcodeScanFinishedWithError("Failed to read input: %s", err)
		}
		projectPath = projpth
	}
	log.Debugf("projectPath: %s", projectPath)
	xcodeCmd := xcode.CommandModel{
		ProjectFilePath: projectPath,
	}

	schemeToUse := paramXcodeScheme
	if schemeToUse == "" {
		fmt.Println()
		fmt.Println()
		log.Println("🔦  Scanning Schemes ...")
		schemes, err := xcodeCmd.ScanSchemes()
		if err != nil {
			return printXcodeScanFinishedWithError("Failed to scan Schemes: %s", err)
		}
		log.Debugf("schemes: %v", schemes)

		fmt.Println()
		selectedScheme, err := goinp.SelectFromStrings("Select the Scheme you usually use in Xcode", schemes)
		if err != nil {
			return printXcodeScanFinishedWithError("Failed to select Scheme: %s", err)
		}
		log.Debugf("selected scheme: %v", selectedScheme)
		schemeToUse = selectedScheme
	}
	xcodeCmd.Scheme = schemeToUse

	fmt.Println()
	fmt.Println()
	log.Println("🔦  Running an Xcode Archive, to get all the required code signing settings...")
	codeSigningSettings, xcodebuildOutput, err := xcodeCmd.ScanCodeSigningSettings()
	// save the xcodebuild output into a debug log file
	xcodebuildOutputFilePath := filepath.Join(absExportOutputDirPath, "xcodebuild-output.log")
	{
		log.Infof("  💡  "+colorstring.Yellow("Saving xcodebuild output into file")+": %s", xcodebuildOutputFilePath)
		if logWriteErr := fileutil.WriteStringToFile(xcodebuildOutputFilePath, xcodebuildOutput); logWriteErr != nil {
			log.Errorf("Failed to save xcodebuild output into file (%s), error: %s", xcodebuildOutputFilePath, logWriteErr)
		} else if err != nil {
			log.Infoln(colorstring.Yellow("Please check the logfile (" + xcodebuildOutputFilePath + ") to see what caused the error"))
			log.Infoln(colorstring.Red("and make sure that you can Archive this project from Xcode!"))
			fmt.Println()
			log.Infoln("Open the project:", xcodeCmd.ProjectFilePath)
			log.Infoln("and Archive, using the Scheme:", xcodeCmd.Scheme)
			fmt.Println()
		}
	}
	if err != nil {
		return printXcodeScanFinishedWithError("Failed to detect code signing settings: %s", err)
	}
	log.Debugf("codeSigningSettings: %#v", codeSigningSettings)

	return exportCodeSigningFiles("Xcode", absExportOutputDirPath, codeSigningSettings)
}

func exportProvisioningProfiles(provProfileFileInfos []provprofile.ProvisioningProfileFileInfoModel,
	exportTargetDirPath string) error {

	for _, aProvProfileFileInfo := range provProfileFileInfos {
		log.Infoln("   " + colorstring.Green("Exporting Provisioning Profile:") + " " + aProvProfileFileInfo.ProvisioningProfileInfo.Name)
		log.Infoln("                             UUID: " + aProvProfileFileInfo.ProvisioningProfileInfo.UUID)
		exportFileName := provProfileExportFileName(aProvProfileFileInfo)
		exportPth := filepath.Join(exportTargetDirPath, exportFileName)
		if err := cmdex.RunCommand("cp", aProvProfileFileInfo.Path, exportPth); err != nil {
			return fmt.Errorf("Failed to copy Provisioning Profile (from: %s) (to: %s), error: %s",
				aProvProfileFileInfo.Path, exportPth, err)
		}
	}
	return nil
}

func provProfileExportFileName(provProfileFileInfo provprofile.ProvisioningProfileFileInfoModel) string {
	replaceRexp, err := regexp.Compile("[^A-Za-z0-9_.-]")
	if err != nil {
		log.Warn("Invalid regex, error: %s", err)
		return ""
	}
	safeTitle := replaceRexp.ReplaceAllString(provProfileFileInfo.ProvisioningProfileInfo.Name, "")
	extension := ".mobileprovision"
	if strings.HasSuffix(provProfileFileInfo.Path, ".provisionprofile") {
		extension = ".provisionprofile"
	}

	return provProfileFileInfo.ProvisioningProfileInfo.UUID + "." + safeTitle + extension
}
