// Package provider implements the Terraform provider for NSResolver DNS hosting.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ns-resolver/terraform-provider-nsresolver/internal/client"
)

var _ provider.Provider = &nsresolverProvider{}

type nsresolverProvider struct {
	version string
}

// New returns the provider factory for the given version.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &nsresolverProvider{version: version} }
}

type providerModel struct {
	Token    types.String `tfsdk:"token"`
	Endpoint types.String `tfsdk:"endpoint"`
}

func (p *nsresolverProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "nsresolver"
	resp.Version = p.version
}

func (p *nsresolverProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage DNS zones, records and zone settings hosted on [NSResolver](https://nsresolver.com), " +
			"through the [NSResolver API](https://api.nsresolver.com/).",
		Attributes: map[string]schema.Attribute{
			"token": schema.StringAttribute{
				MarkdownDescription: "Your API token, from the API token section of your NSResolver account page. " +
					"Can also be set with the `NSRESOLVER_TOKEN` environment variable.",
				Optional:  true,
				Sensitive: true,
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "API base URL. Defaults to `" + client.DefaultEndpoint + "`; " +
					"can also be set with `NSRESOLVER_ENDPOINT`. Only needed for testing.",
				Optional: true,
			},
		},
	}
}

func (p *nsresolverProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	token := os.Getenv("NSRESOLVER_TOKEN")
	if !cfg.Token.IsNull() && !cfg.Token.IsUnknown() {
		token = cfg.Token.ValueString()
	}
	endpoint := os.Getenv("NSRESOLVER_ENDPOINT")
	if !cfg.Endpoint.IsNull() && !cfg.Endpoint.IsUnknown() {
		endpoint = cfg.Endpoint.ValueString()
	}
	if token == "" && !cfg.Token.IsUnknown() {
		resp.Diagnostics.AddError("Missing NSResolver API token",
			"Set token in the provider block or the NSRESOLVER_TOKEN environment variable. "+
				"Your token is on your NSResolver account page.")
		return
	}
	c := client.New(endpoint, token, "terraform-provider-nsresolver/"+p.version)
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *nsresolverProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{newZoneResource, newRecordResource}
}

func (p *nsresolverProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{newZoneDataSource, newRecordsDataSource}
}
