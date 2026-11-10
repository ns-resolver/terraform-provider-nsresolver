package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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
	_ resource.Resource                   = &recordResource{}
	_ resource.ResourceWithConfigure      = &recordResource{}
	_ resource.ResourceWithImportState    = &recordResource{}
	_ resource.ResourceWithValidateConfig = &recordResource{}
)

var recordTypes = []string{"A", "AAAA", "CNAME", "MX", "TXT", "SRV", "CAA", "NS"}

func newRecordResource() resource.Resource { return &recordResource{} }

type recordResource struct{ client *client.Client }

type recordModel struct {
	ID       types.String `tfsdk:"id"`
	Zone     types.String `tfsdk:"zone"`
	Type     types.String `tfsdk:"type"`
	Name     types.String `tfsdk:"name"`
	Answer   types.String `tfsdk:"answer"`
	TTL      types.Int64  `tfsdk:"ttl"`
	Priority types.Int64  `tfsdk:"priority"`
	Weight   types.Int64  `tfsdk:"weight"`
	Port     types.Int64  `tfsdk:"port"`
	Flags    types.Int64  `tfsdk:"flags"`
	Tag      types.String `tfsdk:"tag"`
	FQDN     types.String `tfsdk:"fqdn"`
}

func u16(desc string) schema.Int64Attribute {
	return schema.Int64Attribute{MarkdownDescription: desc, Optional: true,
		Validators: []validator.Int64{int64validator.Between(0, 65535)}}
}

func (r *recordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_record"
}

func (r *recordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One DNS record in an NSResolver zone. A name can hold several records of one type " +
			"(e.g. two `A` records for round robin): use one resource per record.\n\n" +
			"Values are compared in NSResolver's stored form, so e.g. `mail.example.net.` and `mail.example.net` " +
			"are the same answer.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The record's id in NSResolver.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"zone": schema.StringAttribute{
				MarkdownDescription: "The zone the record belongs to, e.g. `nsresolver_zone.example.name`. Changing it replaces the record.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Record type: `" + strings.Join(recordTypes, "`, `") + "`. Changing it replaces the record.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringvalidator.OneOf(recordTypes...)},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name relative to the zone, e.g. `www` or `_sip._tcp`; `@` for the zone itself.",
				Required:            true,
			},
			"answer": schema.StringAttribute{
				MarkdownDescription: "The record's value: an IPv4 address (A), IPv6 address (AAAA), hostname (CNAME, MX, SRV, NS; " +
					"`.` for a null MX or SRV), text (TXT, unquoted) or CAA value (e.g. `letsencrypt.org`).",
				Required: true,
			},
			"ttl": schema.Int64Attribute{
				MarkdownDescription: "TTL in seconds, 60–604800. Left out: the zone's default TTL when the record is created.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.Int64{int64validator.Between(60, 604800)},
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"priority": u16("MX and SRV: priority, 0–65535. Required for those types."),
			"weight":   u16("SRV: weight, 0–65535. Required for SRV."),
			"port":     u16("SRV: port, 0–65535. Required for SRV."),
			"flags": schema.Int64Attribute{
				MarkdownDescription: "CAA: flags, `0` or `128` (critical). Defaults to 0 for CAA records.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.Int64{int64validator.OneOf(0, 128)},
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"tag": schema.StringAttribute{
				MarkdownDescription: "CAA: `issue`, `issuewild`, `iodef` or `issuemail`. Required for CAA.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.OneOfCaseInsensitive("issue", "issuewild", "iodef", "issuemail")},
			},
			"fqdn": schema.StringAttribute{
				MarkdownDescription: "The record's full name, e.g. `www.example.com`.",
				Computed:            true,
			},
		},
	}
}

func (r *recordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

// ValidateConfig: the extra fields each type needs, and no others.
func (r *recordResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m recordModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || m.Type.IsUnknown() || m.Type.IsNull() {
		return
	}
	typ := m.Type.ValueString()
	need := map[string][]string{"MX": {"priority"}, "SRV": {"priority", "weight", "port"}, "CAA": {"tag"}}[typ]
	allowed := map[string]bool{}
	for _, f := range need {
		allowed[f] = true
	}
	if typ == "CAA" {
		allowed["flags"] = true
	}
	set := map[string]bool{
		"priority": !m.Priority.IsNull(), "weight": !m.Weight.IsNull(), "port": !m.Port.IsNull(),
		"flags": !m.Flags.IsNull(), "tag": !m.Tag.IsNull(),
	}
	for _, f := range need {
		if !set[f] {
			resp.Diagnostics.AddAttributeError(path.Root(f), "Missing "+f, fmt.Sprintf("%s records need %s.", typ, f))
		}
	}
	for _, f := range []string{"priority", "weight", "port", "flags", "tag"} {
		if set[f] && !allowed[f] {
			resp.Diagnostics.AddAttributeError(path.Root(f), "Unexpected "+f, fmt.Sprintf("%s records don't use %s.", typ, f))
		}
	}
}

func strPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func recordInput(m recordModel) client.RecordInput {
	return client.RecordInput{
		Type: m.Type.ValueString(), Record: strPtr(m.Name), Answer: strPtr(m.Answer), TTL: int64Ptr(m.TTL),
		Priority: int64Ptr(m.Priority), Weight: int64Ptr(m.Weight), Port: int64Ptr(m.Port),
		Flags: int64Ptr(m.Flags), Tag: strPtr(m.Tag),
	}
}

func (r *recordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan recordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rec, err := r.client.CreateRecord(ctx, normalizeZone(plan.Zone.ValueString()), recordInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating record", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, recordState(plan, rec))...)
}

func (r *recordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state recordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid record id", state.ID.ValueString())
		return
	}
	rec, err := r.client.GetRecord(ctx, normalizeZone(state.Zone.ValueString()), id)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading record", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, recordState(state, rec))...)
}

func (r *recordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state recordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, _ := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	rec, err := r.client.UpdateRecord(ctx, normalizeZone(state.Zone.ValueString()), id, recordInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error changing record", err.Error())
		return
	}
	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, recordState(plan, rec))...)
}

func (r *recordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state recordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, _ := strconv.ParseInt(state.ID.ValueString(), 10, 64)
	err := r.client.DeleteRecord(ctx, normalizeZone(state.Zone.ValueString()), id)
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting record", err.Error())
	}
}

// ImportState takes "<zone>/<record id>", e.g. "example.com/1234".
func (r *recordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	zone, id, ok := strings.Cut(req.ID, "/")
	if _, err := strconv.ParseInt(id, 10, 64); !ok || zone == "" || err != nil {
		resp.Diagnostics.AddError("Invalid import id", `Use "<zone>/<record id>", e.g. "example.com/1234". `+
			"Record ids are in the API's responses, e.g. GET https://api.nsresolver.com/v1/<zone>/.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone"), normalizeZone(zone))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// recordState: the state for rec. Where the prior value (configuration or state) means the same as the
// stored value, the prior spelling is kept, so stored-form differences never show up as changes.
func recordState(prior recordModel, rec *client.Record) recordModel {
	zone := normalizeZone(prior.Zone.ValueString())
	keep := func(p types.String, same bool, stored string) types.String {
		if !p.IsNull() && !p.IsUnknown() && same {
			return p
		}
		return types.StringValue(stored)
	}
	n := func(p *int64) types.Int64 {
		if p == nil {
			return types.Int64Null()
		}
		return types.Int64Value(*p)
	}
	answerSame := false
	if !prior.Answer.IsNull() && !prior.Answer.IsUnknown() {
		if rec.Type == "CAA" && rec.Tag != nil {
			answerSame = normalizeCAA(*rec.Tag, prior.Answer.ValueString()) == rec.Answer
		} else {
			answerSame = normalizeAnswer(rec.Type, prior.Answer.ValueString(), zone) == rec.Answer
		}
	}
	m := recordModel{
		ID:       types.StringValue(strconv.FormatInt(rec.ID, 10)),
		Zone:     keep(prior.Zone, true, zone),
		Type:     types.StringValue(rec.Type),
		Name:     keep(prior.Name, normalizeName(prior.Name.ValueString(), zone) == rec.Record, rec.Record),
		Answer:   keep(prior.Answer, answerSame, rec.Answer),
		TTL:      types.Int64Value(rec.TTL),
		Priority: n(rec.Priority),
		Weight:   n(rec.Weight),
		Port:     n(rec.Port),
		Flags:    n(rec.Flags),
		Tag:      types.StringNull(),
		FQDN:     types.StringValue(rec.FQDN),
	}
	if rec.Tag != nil {
		m.Tag = keep(prior.Tag, strings.EqualFold(prior.Tag.ValueString(), *rec.Tag), *rec.Tag)
	}
	if prior.Zone.IsNull() || prior.Zone.IsUnknown() {
		m.Zone = types.StringValue(zone)
	}
	return m
}
