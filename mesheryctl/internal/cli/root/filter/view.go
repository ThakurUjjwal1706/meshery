// Copyright Meshery Authors
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

package filter

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/meshery/meshery/server/models"

	"github.com/meshery/meshery/mesheryctl/internal/cli/pkg/api"
	mesheryctlflags "github.com/meshery/meshery/mesheryctl/internal/cli/pkg/flags"

	"github.com/meshery/meshery/mesheryctl/internal/cli/pkg/viewoutput"
	"github.com/meshery/meshery/mesheryctl/internal/cli/root/config"
	"github.com/meshery/meshery/mesheryctl/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type filterViewFlags struct {
	ViewAllFlag  bool   `json:"all" validate:"boolean"`
	OutputFormat string `json:"output-format" validate:"required,oneof=json yaml"`
	Save         bool   `json:"save" validate:"boolean"`
}

var filterViewFlagsProvided filterViewFlags

var viewCmd = &cobra.Command{
	Use:   "view",
	Short: "View filter(s)",
	Long: `Displays the contents of a specific filter based on name or id.
	Find more information at: https://docs.meshery.io/reference/mesheryctl/filter/view`,
	Example: `
// View the specified WASM filter
// A unique prefix of the name or ID can also be provided. If the prefix is not unique, the first match will be returned.
mesheryctl filter view "[filter-name | ID]"

// View all filter files
mesheryctl filter view --all

// View all filter files in json
mesheryctl filter view --all --output-format json

// View all filter files in json and save it to a file
mesheryctl filter view --all --output-format json -s

// View multi-word named filter files. Multi-word filter names should be enclosed in quotes
mesheryctl filter view "filter name"
        `,
	Args: func(_ *cobra.Command, args []string) error {
		const errMsg = "Usage: mesheryctl filter view [filter-name | ID]\nRun 'mesheryctl filter view --help' to see detailed help message"
		if len(args) > 1 {
			return utils.ErrInvalidArgument(fmt.Errorf("accepts at most 1 arg, received %d\n\n%s", len(args), errMsg))
		}
		if len(args) > 0 && filterViewFlagsProvided.ViewAllFlag {
			return ErrViewAllWithName("view")
		}
		if (len(args) == 0 || args[0] == "") && !filterViewFlagsProvided.ViewAllFlag {
			return utils.ErrInvalidNameOrID(errors.New(errFilterNameOrIDNotProvided))
		}
		return nil
	},
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return mesheryctlflags.ValidateCmdFlags(cmd, &filterViewFlagsProvided)
	},

	RunE: func(cmd *cobra.Command, args []string) error {

		mctlCfg, err := config.GetMesheryCtl(viper.GetViper())
		if err != nil {
			return utils.ErrLoadConfig(err)
		}

		filter := ""
		isID := false
		if len(args) > 0 {
			filter, isID, err = utils.ValidId(mctlCfg.GetBaseMesheryURL(), args[0], "filter")
			if err != nil {
				return utils.ErrInvalidNameOrID(err)
			}
		}

		urlString := ""
		if len(filter) == 0 {
			urlString = "api/filter?pagesize=10000"
		} else if isID {
			// if filter is a valid uuid, then directly fetch the filter
			urlString = "api/filter/" + filter
		} else {
			// else search filter by name
			urlString = "api/filter?pagesize=10000&search=" + url.QueryEscape(filter)
		}

		var selectedFilter *models.MesheryFilter
		var filterPage *models.MesheryFilterPage
		if isID {
			// Fetch single filter
			filter, err := api.Fetch[models.MesheryFilter](urlString)
			if err != nil {
				return err
			}
			selectedFilter = filter
		} else {
			page, err := api.Fetch[models.MesheryFilterPage](urlString)
			if err != nil {
				return err
			}
			filterPage = page

			if !filterViewFlagsProvided.ViewAllFlag {
				matchedFilters := make([]*models.MesheryFilter, 0)
				// 1. Exact whole-name match (prioritizing exact matches)
				for _, f := range filterPage.Filters {
					if f == nil {
						continue
					}
					if f.Name == filter || utils.TrimFileExtension(f.Name) == filter {
						matchedFilters = append(matchedFilters, f)
					}
				}
				// 2. If no exact match, fall back to case-insensitive whole-name match
				if len(matchedFilters) == 0 {
					for _, f := range filterPage.Filters {
						if f == nil {
							continue
						}
						if strings.EqualFold(f.Name, filter) || strings.EqualFold(utils.TrimFileExtension(f.Name), filter) {
							matchedFilters = append(matchedFilters, f)
						}
					}
				}

				if len(matchedFilters) == 0 {
					utils.Log.Info(fmt.Sprintf("filter with name: %q not found", filter))
					return nil
				}

				if len(matchedFilters) == 1 {
					selectedFilter = matchedFilters[0]
				} else {
					selectedFilter, err = selectFilterPrompt(matchedFilters)
					if err != nil {
						return err
					}
				}
			}
		}

		if selectedFilter == nil && !filterViewFlagsProvided.ViewAllFlag {
			return nil
		}

		var data any

		if filterViewFlagsProvided.ViewAllFlag {
			data = filterPage
		} else {
			data = selectedFilter
		}

		savePath := ""

		if filterViewFlagsProvided.Save {
			var fileName string

			if filterViewFlagsProvided.ViewAllFlag {
				fileName = fmt.Sprintf(
					"filters_all.%s",
					strings.ToLower(filterViewFlagsProvided.OutputFormat),
				)
			} else {
				shortID := utils.TruncateID(utils.FormatUUID(selectedFilter.ID, "unknown"))

				sanitizedName := utils.SanitizeFileName(selectedFilter.Name)
				if sanitizedName == "" {
					sanitizedName = shortID
				}

				fileName = fmt.Sprintf(
					"filter_%s_%s.%s",
					sanitizedName,
					shortID,
					strings.ToLower(filterViewFlagsProvided.OutputFormat),
				)
			}

			savePath = filepath.Join(utils.MesheryFolder, fileName)
		}

		return viewoutput.HandleViewOutput(
			filterViewFlagsProvided.OutputFormat,
			data,
			cmd.OutOrStdout(),
			savePath,
		)

	},
}

var runSelectPrompt = utils.RunSelectPrompt

func selectFilterPrompt(filters []*models.MesheryFilter) (*models.MesheryFilter, error) {
	if len(filters) == 0 {
		return nil, nil
	}

	filterOptions := make([]string, len(filters))
	for i, f := range filters {
		if f == nil {
			filterOptions[i] = "N/A"
			continue
		}
		idStr := utils.FormatUUID(f.ID, "N/A")
		ownerStr := "unknown"
		if f.Owner != nil && *f.Owner != "" {
			ownerStr = *f.Owner
		}
		filterOptions[i] = fmt.Sprintf("ID: %s, Name: %s, Owner: %s", idStr, f.Name, ownerStr)
	}

	i, err := runSelectPrompt("Select a filter", filterOptions)
	if err != nil {
		return nil, err
	}

	return filters[i], nil
}

func init() {
	viewCmd.Flags().BoolVarP(&filterViewFlagsProvided.ViewAllFlag, "all", "a", false, "(optional) view all filters available")
	viewCmd.Flags().StringVarP(&filterViewFlagsProvided.OutputFormat, "output-format", "o", "yaml", "(optional) format to display in [json|yaml]")
	viewCmd.Flags().BoolVarP(&filterViewFlagsProvided.Save, "save", "s", false, "(optional) save output as a JSON/YAML file")
}
