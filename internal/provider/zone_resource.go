package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ns-resolver/terraform-provider-nsresolver/internal/client"
)

var (
	_ resource.Resource                   = &zoneResource{}
	_ resource.ResourceWithConfigure      = &zoneResource{}
	_ resource.ResourceWithImportState    = &zoneResource{}
	_ resource.ResourceWithValidateConfig = &zoneResource{}
)

var zoneNameRe = regexp.MustCompile(`^(?i)[a-z0-9_-]+(\.[a-z0-9_-]+)+\.?$`)

func newZoneResource() resource.Resource { return &zoneResource{} }

type zoneResource struct{ client *client.Client }

type zoneModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	DefaultTTL     types.Int64  `tfsdk:"default_ttl"`
	Refresh        types.Int64  `tfsdk:"refresh"`
	Retry          types.Int64  `tfsdk:"retry"`
	Expire         types.Int64  `tfsdk:"expire"`
	NegativeAnswer types.Int64  `tfsdk:"negative_answer"`
	Status         types.String `tfsdk:"status"`
}

// The API's limits for the settings (see "Zone settings" in the API docs).
func settingAttr(desc string, min, max int64) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: fmt.Sprintf("%s In seconds, %d–%d. Left out: the current value is kept (for a new zone, the default).", desc, min, max),
		Optional:            true,
		Computed:            true,
		Validators:          []validator.Int64{int64validator.Between(min, max)},
		PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
	}
}

func (r *zoneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_zone"
}

func (r *zoneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A DNS zone (domain) in your NSResolver account, with its settings. " +
			"After creating it, point your domain's nameservers at NSResolver at your registrar.\n\n" +
			"~> Deleting a zone deletes **all** its records, including any not managed by Terraform, " +
			"and NSResolver's nameservers stop answering for the domain.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The zone name.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The domain, e.g. `example.com`. Changing it replaces the zone. " +
					"A domain that is already in another NSResolver account can't be added.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{stringvalidator.RegexMatches(zoneNameRe,
					"must be a domain name such as example.com")},
			},
			"default_ttl":     settingAttr("TTL used for records created without one.", 60, 604800),
			"refresh":         settingAttr("SOA refresh.", 1200, 86400),
			"retry":           settingAttr("SOA retry; must be less than `refresh`.", 600, 43200),
			"expire":          settingAttr("SOA expire.", 604800, 2419200),
			"negative_answer": settingAttr("How long resolvers cache \"no such name\" (the SOA negative-answer TTL).", 60, 10800),
			"status": schema.StringAttribute{
				MarkdownDescription: "`live` (the registry lists NSResolver's nameservers), `pending` (not yet; the zone " +
					"is served anyway) or `on hold` (waiting for a review by NSResolver, because the domain was recently " +
					"in another account).",
				Computed: true,
			},
		},
	}
}

func (r *zoneResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *zoneResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m zoneModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if known(m.Retry) && known(m.Refresh) && m.Retry.ValueInt64() >= m.Refresh.ValueInt64() {
		resp.Diagnostics.AddAttributeError(path.Root("retry"), "Invalid retry", "retry must be less than refresh.")
	}
}

func known(v types.Int64) bool { return !v.IsNull() && !v.IsUnknown() }

func int64Ptr(v types.Int64) *int64 {
	if !known(v) {
		return nil
	}
	x := v.ValueInt64()
	return &x
}

// wantedSettings: the settings set in the plan (nil = not set).
func wantedSettings(m zoneModel) client.Settings {
	return client.Settings{
		DefaultTTL: int64Ptr(m.DefaultTTL), Refresh: int64Ptr(m.Refresh), Retry: int64Ptr(m.Retry),
		Expire: int64Ptr(m.Expire), NegativeAnswer: int64Ptr(m.NegativeAnswer),
	}
}

func differs(want *int64, have *int64) bool { return want != nil && (have == nil || *want != *have) }

func (r *zoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan zoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	z, err := r.client.CreateZone(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating zone", err.Error())
		return
	}
	want, have := wantedSettings(plan), z.Settings
	if differs(want.DefaultTTL, have.DefaultTTL) || differs(want.Refresh, have.Refresh) || differs(want.Retry, have.Retry) ||
		differs(want.Expire, have.Expire) || differs(want.NegativeAnswer, have.NegativeAnswer) {
		s, err := r.client.SetSettings(ctx, z.Zone, want)
		if err != nil {
			// Don't leave a half-configured zone behind.
			if derr := r.client.DeleteZone(ctx, z.Zone); derr != nil {
				err = fmt.Errorf("%w (and removing the new zone again failed: %v)", err, derr)
			}
			resp.Diagnostics.AddError("Error applying zone settings", err.Error())
			return
		}
		z.Settings = *s
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, zoneState(plan, z))...)
}

func (r *zoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state zoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	z, _, err := r.client.GetZone(ctx, normalizeZone(state.Name.ValueString()))
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading zone", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, zoneState(state, z))...)
}

func (r *zoneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan zoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name := normalizeZone(plan.Name.ValueString())
	if _, err := r.client.SetSettings(ctx, name, wantedSettings(plan)); err != nil {
		resp.Diagnostics.AddError("Error changing zone settings", err.Error())
		return
	}
	z, _, err := r.client.GetZone(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Error reading zone", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, zoneState(plan, z))...)
}

func (r *zoneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state zoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteZone(ctx, normalizeZone(state.Name.ValueString())); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting zone", err.Error())
	}
}

func (r *zoneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), normalizeZone(req.ID))...)
}

// zoneState: the state for zone z. The name keeps the configuration's spelling when it means the same.
func zoneState(prior zoneModel, z *client.Zone) zoneModel {
	name := z.Zone
	if !prior.Name.IsNull() && normalizeZone(prior.Name.ValueString()) == z.Zone {
		name = prior.Name.ValueString()
	}
	v := func(p *int64) types.Int64 {
		if p == nil {
			return types.Int64Null()
		}
		return types.Int64Value(*p)
	}
	return zoneModel{
		ID: types.StringValue(z.Zone), Name: types.StringValue(name),
		DefaultTTL: v(z.Settings.DefaultTTL), Refresh: v(z.Settings.Refresh), Retry: v(z.Settings.Retry),
		Expire: v(z.Settings.Expire), NegativeAnswer: v(z.Settings.NegativeAnswer),
		Status: types.StringValue(z.Status),
	}
}
