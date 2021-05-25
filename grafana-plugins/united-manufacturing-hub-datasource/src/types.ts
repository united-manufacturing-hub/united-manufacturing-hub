import { DataQuery, DataSourceJsonData } from '@grafana/data';

export interface MyQuery extends DataQuery {
  location: { label: string; index: number };
  asset: { label: string; index: number };
  value: { label: string; index: number };
  parameterString: string;
  labelsField: string;
  isCommand: boolean;
}

export const defaultQuery: Partial<MyQuery> = {
  location: { label: '', index: 0 },
  asset: { label: '', index: 0 },
  value: { label: '', index: 0 },
  parameterString: '',
  labelsField: '',
  isCommand: false,
};

/**
 * These are options configured for each DataSource instance
 */
export interface MyDataSourceOptions extends DataSourceJsonData {
  historicalEnabled: boolean;
  serverURL: string;
  customerId: string;

  realtimeEnabled: boolean;
  brokerURI: string;
  secureConnectionMQTT: boolean;

  // Variables to store the last query
  lastLocationIndex: number;
  lastAssetIndex: number;
  lastValueIndex: number;
}

/**
 * Value that is used in the backend, but never sent over HTTP to the frontend
 */
export interface MySecureJsonData {
  apiKey?: string;
  CACert?: string;
  privkey?: string;
}
