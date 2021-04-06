import { DataSourcePlugin } from '@grafana/data';
import { DataSource } from './DataSource';
import { ConfigEditor } from './ConfigEditor';
import { QueryEditor } from './QueryEditor';
import { JSONQuery, JSONQueryOptions } from './types';

export const plugin = new DataSourcePlugin<DataSource, JSONQuery, JSONQueryOptions>(DataSource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);
