package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &teamRepositoryResource{}
	_ resource.ResourceWithConfigure   = &teamRepositoryResource{}
	_ resource.ResourceWithImportState = &teamRepositoryResource{}
)

// teamRepositoryResource is the resource implementation.
type teamRepositoryResource struct {
	client *forgejo.Client
}

// teamRepositoryResourceModel maps the resource schema data.
type teamRepositoryResourceModel struct {
	TeamID     types.Int64  `tfsdk:"team_id"`
	Owner      types.String `tfsdk:"owner"`
	Repository types.String `tfsdk:"repository"`
}

// Metadata returns the resource type name.
func (r *teamRepositoryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_repository"
}

// Schema defines the schema for the resource.
func (r *teamRepositoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Forgejo team repository resource.",

		Attributes: map[string]schema.Attribute{
			"team_id": schema.Int64Attribute{
				Description: "Numeric identifier of the team. Changing this forces a new resource to be created.",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"owner": schema.StringAttribute{
				Description: "Owner (organization or user) of the repository. Changing this forces a new resource to be created.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"repository": schema.StringAttribute{
				Description: "Name of the repository. Changing this forces a new resource to be created.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *teamRepositoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*forgejo.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf(
				"Expected *forgejo.Client, got: %T. Please report this issue to the provider developers.",
				req.ProviderData,
			),
		)

		return
	}

	r.client = client
}

// Create creates the resource and sets the initial Terraform state.
func (r *teamRepositoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer un(trace(ctx, "Create team repository resource"))

	var data teamRepositoryResourceModel

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = addTeamRepository(
		ctx,
		r.client,
		data.TeamID.ValueInt64(),
		data.Owner.ValueString(),
		data.Repository.ValueString(),
	)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data.
func (r *teamRepositoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer un(trace(ctx, "Read team repository resource"))

	var data teamRepositoryResourceModel

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = checkTeamRepository(
		ctx,
		r.client,
		data.TeamID.ValueInt64(),
		data.Owner.ValueString(),
		data.Repository.ValueString(),
	)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *teamRepositoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	defer un(trace(ctx, "Update team repository resource"))

	// All writable attributes have RequiresReplace set; no in-place update possible.
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *teamRepositoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	defer un(trace(ctx, "Delete team repository resource"))

	var data teamRepositoryResourceModel

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = removeTeamRepository(
		ctx,
		r.client,
		data.TeamID.ValueInt64(),
		data.Owner.ValueString(),
		data.Repository.ValueString(),
	)
	resp.Diagnostics.Append(diags...)
}

// ImportState handles import of an existing team repository into Terraform state.
// The import ID must be in the format "team_id/owner/repository".
func (r *teamRepositoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	defer un(trace(ctx, "Import team repository resource"))

	parts := strings.SplitN(req.ID, "/", 3)
	if len(parts) != 3 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format 'team_id/owner/repository', got: %q", req.ID),
		)
		return
	}

	teamID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid team_id in import ID",
			fmt.Sprintf("team_id must be a numeric value, got: %q: %s", parts[0], err),
		)
		return
	}

	state := teamRepositoryResourceModel{
		TeamID:     types.Int64Value(teamID),
		Owner:      types.StringValue(parts[1]),
		Repository: types.StringValue(parts[2]),
	}

	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// NewTeamRepositoryResource is a helper function to simplify the provider implementation.
func NewTeamRepositoryResource() resource.Resource {
	return &teamRepositoryResource{}
}

// checkTeamRepository verifies that a repository belongs to a team.
func checkTeamRepository(ctx context.Context, client *forgejo.Client, teamID int64, owner, repo string) diag.Diagnostics {
	var diags diag.Diagnostics

	tflog.Info(ctx, "Read team repository", map[string]any{
		"team_id": teamID,
		"owner":   owner,
		"repo":    repo,
	})

	repos, res, err := client.ListTeamRepositories(teamID, forgejo.ListTeamRepositoriesOptions{})
	if err != nil {
		var msg string
		if res == nil {
			msg = fmt.Sprintf("Unknown error with nil response: %s", err)
		} else {
			tflog.Error(ctx, "Error", map[string]any{"status": res.Status})
			msg = fmt.Sprintf("Unknown error (status %d): %s", res.StatusCode, err)
		}
		diags.AddError("Unable to read team repositories", msg)
		return diags
	}

	for _, r := range repos {
		if r.Name == repo {
			return diags
		}
	}

	diags.AddError(
		"Unable to read team repository",
		fmt.Sprintf("Repository '%s/%s' not found in team with ID %d", owner, repo, teamID),
	)

	return diags
}

// addTeamRepository adds a repository to a team.
func addTeamRepository(ctx context.Context, client *forgejo.Client, teamID int64, owner, repo string) diag.Diagnostics {
	var diags diag.Diagnostics

	tflog.Info(ctx, "Create team repository", map[string]any{
		"team_id": teamID,
		"owner":   owner,
		"repo":    repo,
	})

	res, err := client.AddTeamRepository(teamID, owner, repo)
	if err == nil {
		return diags
	}

	var msg string
	if res == nil {
		msg = fmt.Sprintf("Unknown error with nil response: %s", err)
	} else {
		tflog.Error(ctx, "Error", map[string]any{"status": res.Status})

		switch res.StatusCode {
		case 403:
			msg = fmt.Sprintf(
				"Forbidden: team with ID %d does not have permission to access '%s/%s': %s",
				teamID, owner, repo, err,
			)
		case 404:
			msg = fmt.Sprintf(
				"Either repository '%s/%s' or team with ID %d not found: %s",
				owner, repo, teamID, err,
			)
		default:
			msg = fmt.Sprintf("Unknown error (status %d): %s", res.StatusCode, err)
		}
	}
	diags.AddError("Unable to create team repository", msg)

	return diags
}

// removeTeamRepository removes a repository from a team.
func removeTeamRepository(ctx context.Context, client *forgejo.Client, teamID int64, owner, repo string) diag.Diagnostics {
	var diags diag.Diagnostics

	tflog.Info(ctx, "Delete team repository", map[string]any{
		"team_id": teamID,
		"owner":   owner,
		"repo":    repo,
	})

	res, err := client.RemoveTeamRepository(teamID, owner, repo)
	if err == nil {
		return diags
	}

	var msg string
	if res == nil {
		msg = fmt.Sprintf("Unknown error with nil response: %s", err)
	} else {
		tflog.Error(ctx, "Error", map[string]any{"status": res.Status})

		switch res.StatusCode {
		case 404:
			msg = fmt.Sprintf(
				"Either repository '%s/%s' or team with ID %d not found: %s",
				owner, repo, teamID, err,
			)
		default:
			msg = fmt.Sprintf("Unknown error (status %d): %s", res.StatusCode, err)
		}
	}
	diags.AddError("Unable to delete team repository", msg)

	return diags
}
