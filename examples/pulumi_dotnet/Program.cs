using Pulumi;
using Pulumi.Cape.Schemas.Inputs;
using Pulumi.Cape.Storage;
using Pulumi.Cape.Workspace;

return await Deployment.RunAsync(() =>
{
    var ws = new Workspace("myWorkspace", new()
    {
        Spec = new()
    });
    var bs = new BlockStorage("myStorage", new()
    {
        Spec = new BlockStorageSpecArgs
        {
            SizeGB = 32,
            SkuRef = new ReferenceArgs
            {
                Resource = "my-SKU-reference",
            },
        },
        Workspace = ws.Metadata.Apply(m => m.Name),
    });
});
