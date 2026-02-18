import * as cape from '@pulumi/cape';

const ws = new cape.workspace.Workspace('myWorkspace', { spec: {} }, {});
const bs = new cape.storage.BlockStorage('myStorage', {
  workspace: ws.metadata.apply((m) => m.name),
  spec: {
    sizeGB: 32,
    skuRef: {
      resource: 'skus/standard',
    },
  },
});
