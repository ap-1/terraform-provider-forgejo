package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"
)

var (
	_ resource.Resource              = &repositoryWebhookResource{}
	_ resource.ResourceWithConfigure = &repositoryWebhookResource{}
)

type repositoryWebhookResource struct {
	client *forgejo.Client
}

type repositoryWebhookResourceModel struct {
	RepositoryID types.Int64  `tfsdk:"repository_id"`
	HookID       types.Int64  `tfsdk:"hook_id"`
	Type         types.String `tfsdk:"type"`
	URL          types.String `tfsdk:"url"`
	Secret       types.String `tfsdk:"secret"`
	ContentType  types.String `tfsdk:"content_type"`
	Active       types.Bool   `tfsdk:"active"`
	Events       types.List   `tfsdk:"events"`
	CreatedAt    types.String `tfsdk:"created_at"`
}

func (m *repositoryWebhookResourceModel) from(ctx context.Context, h *forgejo.Hook) {
	if h == nil {
		return
	}

	m.HookID = types.Int64Value(h.ID)
	m.Type = types.StringValue(string(h.Type))
	m.URL = types.StringValue(h.Config["url"])
	m.ContentType = types.StringValue(h.Config["content_type"])
	m.Active = types.BoolValue(h.Active)
	m.CreatedAt = types.StringValue(h.Created.Format(time.RFC3339))

	events, _ := types.ListValueFrom(ctx, types.StringType, h.Events)
	m.Events = events
}

func (r *repositoryWebhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_repository_webhook"
}

func (r *repositoryWebhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Forgejo repository webhook resource.",

		Attributes: map[string]schema.Attribute{
			"repository_id": schema.Int64Attribute{
				Description: "Numeric identifier of the repository.",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"hook_id": schema.Int64Attribute{
				Description: "Numeric identifier of the webhook.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				Description: "Type of the webhook (forgejo, slack, discord, etc.).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"url": schema.StringAttribute{
				Description: "Target URL for the webhook.",
				Required:    true,
			},
			"secret": schema.StringAttribute{
				Description: "Secret used to sign webhook payloads.",
				Optional:    true,
				Sensitive:   true,
			},
			"content_type": schema.StringAttribute{
				Description: "Content type for the webhook payload (json or form).",
				Optional:    true,
			},
			"active": schema.BoolAttribute{
				Description: "Whether the webhook is active.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"events": schema.ListAttribute{
				Description: "List of events that trigger the webhook.",
				Required:    true,
				ElementType: types.StringType,
			},
			"created_at": schema.StringAttribute{
				Description: "Time at which the webhook was created.",
				Computed:    true,
			},
		},
	}
}

func (r *repositoryWebhookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *repositoryWebhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer un(trace(ctx, "Create repository webhook resource"))

	var (
		repo repositoryResourceModel
		data repositoryWebhookResourceModel
	)

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	rep, diags := getRepositoryByID(ctx, r.client, data.RepositoryID.ValueInt64())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	repo.from(rep)

	var events []string
	diags = data.Events.ElementsAs(ctx, &events, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := map[string]string{
		"url": data.URL.ValueString(),
	}
	if !data.ContentType.IsNull() {
		config["content_type"] = data.ContentType.ValueString()
	}
	if !data.Secret.IsNull() {
		config["secret"] = data.Secret.ValueString()
	}

	opt := forgejo.CreateHookOption{
		Type:   forgejo.HookType(data.Type.ValueString()),
		Config: config,
		Events: events,
		Active: data.Active.ValueBool(),
	}

	tflog.Info(ctx, "Create repository webhook", map[string]any{
		"user": repo.Owner.ValueString(),
		"repo": repo.Name.ValueString(),
		"url":  data.URL.ValueString(),
		"type": data.Type.ValueString(),
	})

	hook, _, err := r.client.CreateRepoHook(
		repo.Owner.ValueString(),
		repo.Name.ValueString(),
		opt,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating repository webhook",
			fmt.Sprintf("Could not create webhook: %s", err),
		)
		return
	}

	data.from(ctx, hook)

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *repositoryWebhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer un(trace(ctx, "Read repository webhook resource"))

	var (
		repo repositoryResourceModel
		data repositoryWebhookResourceModel
	)

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	rep, diags := getRepositoryByID(ctx, r.client, data.RepositoryID.ValueInt64())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	repo.from(rep)

	hook, _, err := r.client.GetRepoHook(
		repo.Owner.ValueString(),
		repo.Name.ValueString(),
		data.HookID.ValueInt64(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading repository webhook",
			fmt.Sprintf("Could not read webhook: %s", err),
		)
		return
	}

	data.from(ctx, hook)

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *repositoryWebhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	defer un(trace(ctx, "Update repository webhook resource"))

	var (
		repo repositoryResourceModel
		data repositoryWebhookResourceModel
	)

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	rep, diags := getRepositoryByID(ctx, r.client, data.RepositoryID.ValueInt64())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	repo.from(rep)

	var events []string
	diags = data.Events.ElementsAs(ctx, &events, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := map[string]string{
		"url": data.URL.ValueString(),
	}
	if !data.ContentType.IsNull() {
		config["content_type"] = data.ContentType.ValueString()
	}
	if !data.Secret.IsNull() {
		config["secret"] = data.Secret.ValueString()
	}

	active := data.Active.ValueBool()
	opt := forgejo.EditHookOption{
		Config: config,
		Events: events,
		Active: &active,
	}

	_, err := r.client.EditRepoHook(
		repo.Owner.ValueString(),
		repo.Name.ValueString(),
		data.HookID.ValueInt64(),
		opt,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating repository webhook",
			fmt.Sprintf("Could not update webhook: %s", err),
		)
		return
	}

	hook, _, err := r.client.GetRepoHook(
		repo.Owner.ValueString(),
		repo.Name.ValueString(),
		data.HookID.ValueInt64(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading repository webhook after update",
			fmt.Sprintf("Could not read webhook: %s", err),
		)
		return
	}

	data.from(ctx, hook)

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
}

func (r *repositoryWebhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	defer un(trace(ctx, "Delete repository webhook resource"))

	var (
		repo repositoryResourceModel
		data repositoryWebhookResourceModel
	)

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	rep, diags := getRepositoryByID(ctx, r.client, data.RepositoryID.ValueInt64())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	repo.from(rep)

	tflog.Info(ctx, "Delete repository webhook", map[string]any{
		"user":    repo.Owner.ValueString(),
		"repo":    repo.Name.ValueString(),
		"hook_id": data.HookID.ValueInt64(),
	})

	_, err := r.client.DeleteRepoHook(
		repo.Owner.ValueString(),
		repo.Name.ValueString(),
		data.HookID.ValueInt64(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting repository webhook",
			fmt.Sprintf("Could not delete webhook: %s", err),
		)
		return
	}
}

// NewRepositoryWebhookResource is a helper function to simplify the provider implementation.
func NewRepositoryWebhookResource() resource.Resource {
	return &repositoryWebhookResource{}
}
