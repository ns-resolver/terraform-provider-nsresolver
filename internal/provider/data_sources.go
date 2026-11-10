package provider

import (
	"context"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ns-resolver/terraform-provider-nsresolver/internal/client"
)

// ---- nsresolver_zone ----

func newZoneDataSource() datasource.DataSource { return &zoneDataSource{} }

type zoneDataSource struct{ client *client.Client }

type zoneDataModel struct {
	Name           types.String `tfsdk:"name"`
	ID             types.String `tfsdk:"id"`
	Status         types.String `tfsdk:"status"`
	Published      types.Bool   `tfsdk:"published"`
	Serial         types.Int64  `tfsdk:"serial"`
	RecordCount    types.Int64  `tfsdk:"record_count"`
	DefaultTTL     types.Int64  `tfsdk:"default_ttl"`
	Refresh        types.Int64  `tfsdk:"refresh"`
	Retry          types.Int64  `tfsdk:"retry"`
	Expire         types.Int64  `tfsdk:"expire"`
	NegativeAnswer types.Int64  `tfsdk:"negative_answer"`
}

func (d *zoneDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_zone"
}

func (d *zoneDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	c := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{MarkdownDescription: desc, Computed: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a zone in your NSResolver account.",
		Attributes: map[string]schema.Attribute{
			"name":            schema.StringAttribute{MarkdownDescription: "The domain, e.g. `example.com`.", Required: true},
			"id":              schema.StringAttribute{MarkdownDescription: "The zone name.", Computed: true},
			"status":          schema.StringAttribute{MarkdownDescription: "`live`, `pending` or `on hold`.", Computed: true},
			"published":       schema.BoolAttribute{MarkdownDescription: "Whether NSResolver's nameservers serve the latest change.", Computed: true},
			"serial":          c("The zone's SOA serial."),
			"record_count":    c("Number of records in the zone."),
			"default_ttl":     c("TTL used for records created without one, in seconds."),
			"refresh":         c("SOA refresh, in seconds."),
			"retry":           c("SOA retry, in seconds."),
			"expire":          c("SOA expire, in seconds."),
			"negative_answer": c("SOA negative-answer TTL, in seconds."),
		},
	}
}

func (d *zoneDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *zoneDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m zoneDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	z, _, err := d.client.GetZone(ctx, normalizeZone(m.Name.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading zone", err.Error())
		return
	}
	v := func(p *int64) types.Int64 {
		if p == nil {
			return types.Int64Null()
		}
		return types.Int64Value(*p)
	}
	m.ID, m.Status, m.Published = types.StringValue(z.Zone), types.StringValue(z.Status), types.BoolValue(z.Published)
	m.Serial, m.RecordCount = types.Int64Value(z.Serial), types.Int64Value(z.Records)
	m.DefaultTTL, m.Refresh, m.Retry = v(z.Settings.DefaultTTL), v(z.Settings.Refresh), v(z.Settings.Retry)
	m.Expire, m.NegativeAnswer = v(z.Settings.Expire), v(z.Settings.NegativeAnswer)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// ---- nsresolver_records ----

func newRecordsDataSource() datasource.DataSource { return &recordsDataSource{} }

type recordsDataSource struct{ client *client.Client }

type recordsDataModel struct {
	Zone    types.String      `tfsdk:"zone"`
	Type    types.String      `tfsdk:"type"`
	Name    types.String      `tfsdk:"name"`
	ID      types.String      `tfsdk:"id"`
	Records []recordDataModel `tfsdk:"records"`
}

type recordDataModel struct {
	ID       types.String `tfsdk:"id"`
	Type     types.String `tfsdk:"type"`
	Name     types.String `tfsdk:"name"`
	FQDN     types.String `tfsdk:"fqdn"`
	Answer   types.String `tfsdk:"answer"`
	TTL      types.Int64  `tfsdk:"ttl"`
	Priority types.Int64  `tfsdk:"priority"`
	Weight   types.Int64  `tfsdk:"weight"`
	Port     types.Int64  `tfsdk:"port"`
	Flags    types.Int64  `tfsdk:"flags"`
	Tag      types.String `tfsdk:"tag"`
}

func (d *recordsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_records"
}

func (d *recordsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	s := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{MarkdownDescription: desc, Computed: true}
	}
	i := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{MarkdownDescription: desc, Computed: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the records in an NSResolver zone, optionally only those of one type and/or name. " +
			"Useful for finding record ids to import.",
		Attributes: map[string]schema.Attribute{
			"zone": schema.StringAttribute{MarkdownDescription: "The zone, e.g. `example.com`.", Required: true},
			"type": schema.StringAttribute{MarkdownDescription: "Only records of this type, e.g. `A`.", Optional: true},
			"name": schema.StringAttribute{MarkdownDescription: "Only records with this name, e.g. `www` or `@`.", Optional: true},
			"id":   schema.StringAttribute{MarkdownDescription: "The zone name.", Computed: true},
			"records": schema.ListNestedAttribute{
				MarkdownDescription: "The matching records, in the API's order.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":   s("Record id (for `terraform import nsresolver_record.x <zone>/<id>`)."),
					"type": s("Record type."), "name": s("Name relative to the zone; `@` for the zone itself."),
					"fqdn": s("Full name."), "answer": s("The value."), "ttl": i("TTL in seconds."),
					"priority": i("MX/SRV priority."), "weight": i("SRV weight."), "port": i("SRV port."),
					"flags": i("CAA flags."), "tag": s("CAA tag."),
				}},
			},
		},
	}
}

func (d *recordsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		d.client = c
	}
}

func (d *recordsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m recordsDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	zone := normalizeZone(m.Zone.ValueString())
	z, recs, err := d.client.GetZone(ctx, zone)
	if err != nil {
		resp.Diagnostics.AddError("Error reading zone", err.Error())
		return
	}
	n := func(p *int64) types.Int64 {
		if p == nil {
			return types.Int64Null()
		}
		return types.Int64Value(*p)
	}
	m.ID = types.StringValue(z.Zone)
	m.Records = []recordDataModel{}
	for _, r := range recs {
		if !m.Type.IsNull() && !strings.EqualFold(m.Type.ValueString(), r.Type) {
			continue
		}
		if !m.Name.IsNull() && normalizeName(m.Name.ValueString(), zone) != r.Record {
			continue
		}
		tag := types.StringNull()
		if r.Tag != nil {
			tag = types.StringValue(*r.Tag)
		}
		m.Records = append(m.Records, recordDataModel{
			ID: types.StringValue(strconv.FormatInt(r.ID, 10)), Type: types.StringValue(r.Type),
			Name: types.StringValue(r.Record), FQDN: types.StringValue(r.FQDN), Answer: types.StringValue(r.Answer),
			TTL: types.Int64Value(r.TTL), Priority: n(r.Priority), Weight: n(r.Weight), Port: n(r.Port),
			Flags: n(r.Flags), Tag: tag,
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
