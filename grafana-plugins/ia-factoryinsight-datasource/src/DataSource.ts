import defaults from 'lodash/defaults';
import { isUndefined, isString } from 'lodash';

import {
  DataQueryRequest,
  DataQueryResponse,
  DataSourceApi,
  DataSourceInstanceSettings,
  MutableDataFrame,
  FieldType,
  ScopedVars,
} from '@grafana/data';

import { getTemplateSrv } from '@grafana/runtime';

import { JSONQuery, JSONQueryOptions, defaultQuery } from './types';

export class DataSource extends DataSourceApi<JSONQuery, JSONQueryOptions> {
  // To hold the instance's configuration parameters
  baseURL: string;
  apiPath = '/factoryinsight/api/v1/'; // Hardcoded for now
  customerId: string;

  // Grafana's backend
  backendSrv: any;

  // Template variables
  template_location: string;
  template_asset: string;
  template_value: string;

  constructor(instanceSettings: DataSourceInstanceSettings<JSONQueryOptions>, backendSrv: any) {
    super(instanceSettings);
    this.customerId = instanceSettings.jsonData.customerId || '';
    this.baseURL = (instanceSettings.url || '') + this.apiPath + this.customerId;
    this.backendSrv = backendSrv;

    // Initialise template variables
    this.template_location = '';
    this.template_asset = '';
    this.template_value = '';
  }

  async query(options: DataQueryRequest<JSONQuery>): Promise<DataQueryResponse> {
    const { range } = options;
    const from = range!.from.valueOf();
    const to = range!.to.valueOf();
    const from_ISO = new Date(from).toISOString();
    const to_ISO = new Date(to).toISOString();

    // Template variables
    this.template_location = this.getTemplateVariable('location', options.scopedVars);
    this.template_asset = this.getTemplateVariable('asset', options.scopedVars);
    this.template_value = this.getTemplateVariable('value', options.scopedVars);

    // Get from the server the data format and the data points that correspond
    // to the selected value and time period
    const resultArray = await this.getDatapoints(from_ISO, to_ISO, options.targets);

    // Temporary space for the requested datapoints, dataformat and requested value
    // Initialising array
    const datapoints = resultArray.datapoints;
    const columnNames = resultArray.columnNames;

    // Return a constant for each query.
    const data = options.targets.map((target, queryIndex) => {
      const query = defaults(target, defaultQuery);

      // Return and emtpy frame if no location, asset or value has been specificied
      const frame = new MutableDataFrame({
        refId: query.refId,
        fields: [],
      });

      if (this.isEmptyOrUndefined(query.location.label)) {
        return frame;
      }
      if (this.isEmptyOrUndefined(query.asset.label)) {
        return frame;
      }
      if (this.isEmptyOrUndefined(query.value.label)) {
        return frame;
      }

      // Handle empty arrays
      if (isUndefined(datapoints[queryIndex])) {
        return frame;
      }

      // Turn rows into fields if defined by user
      if (query.labelsField !== undefined && query.labelsField !== '') {
        const fieldNameIndex = columnNames[queryIndex].indexOf(query.labelsField);

        if (fieldNameIndex === -1) {
          console.error(`ERROR: Column ${query.labelsField} not found. Using default format.`);
        } else {
          // This are the new column names
          const newColumnNames = datapoints[queryIndex][fieldNameIndex];
          // Filter out the column names from the table and transpose it for easier assignment
          const newDatapoints = this.transpose(
            datapoints[queryIndex].filter((element, eIndex) => {
              return eIndex !== fieldNameIndex;
            })
          );
          // Create a new field with the corresponding data
          newColumnNames.map((columnName, columnIndex) => {
            frame.addField({
              name: columnName.toString(),
              type: FieldType.number,
              values: newDatapoints[columnIndex],
            });
          });

          return frame;
        }
      }

      // If no label column was specified, handle the incoming data with the
      // defined data model structure:
      // { columnNames: string[], datapoints: any[][] }
      columnNames[queryIndex].map((columnName, columnIndex) => {
        // Look for the fixed columns
        if (columnName === 'timestamp') {
          frame.addField({
            name: columnName,
            type: FieldType.time,
            values: datapoints[queryIndex][columnIndex],
          });
        } else if (columnName === 'fieldName') {
          // TODO Special case
        } else {
          // Check data type
          const sampleValue = datapoints[queryIndex][columnIndex][0];
          frame.addField({
            name: columnName,
            type: isString(sampleValue) ? FieldType.string : FieldType.number,
            values: datapoints[queryIndex][columnIndex],
          });
        }
      });

      return frame;
    });

    return { data };
  }

  async testDatasource() {
    // Implement a health check for your data source.
    let testResult = {
      status: 'success',
      message: 'Data source works.',
      title: 'Success',
    };
    await this.backendSrv
      .datasourceRequest({
        url: `${this.baseURL}`,
        method: 'GET',
      })
      .then((res: any) => {
        if (res.status !== 200) {
          testResult.status = 'error';
          testResult.message = `Wrong response from server: ${res.message}`;
          testResult.title = `Data source connection error ${res.status}`;
        }
      })
      .catch((error: any) => {
        testResult.status = 'error';
        testResult.message = `Caught error in datasource test: ${error}`;
        testResult.title = `Data source exception`;
      });

    return testResult;
  }

  async getLocations(callback: Function) {
    this.backendSrv
      .datasourceRequest({
        url: this.baseURL,
        method: 'GET',
      })
      .then((res: any) => {
        callback(res.data);
      });
  }

  async getAssets(location: string, callback: Function) {
    if (location === '' || isUndefined(location)) {
      return;
    }
    this.backendSrv
      .datasourceRequest({
        url: this.baseURL + `/${location}`,
        method: 'GET',
      })
      .then((res: any) => {
        callback(res.data);
      });
  }

  async getValues(location: string, asset: string, callback: Function) {
    if (location === '' || isUndefined(location)) {
      return;
    }
    if (asset === '' || isUndefined(asset)) {
      return;
    }
    this.backendSrv
      .datasourceRequest({
        url: this.baseURL + `/${location}/${asset}`,
        method: 'GET',
      })
      .then((res: any) => {
        callback(res.data);
      });
  }

  getTemplateVariable = (name: string, scopedVars: ScopedVars | undefined) => {
    let template = '${' + name + '}';
    let var_value = getTemplateSrv().replace(template, scopedVars);
    return var_value === template ? '' : var_value;
  };

  isEmptyOrUndefined = (input_string: string) => {
    return input_string === '' || isUndefined(input_string);
  };

  transpose = (a: number[][]) => {
    // Calculate the width and height of the Array
    var w = a.length || 0;
    var h = a[0] instanceof Array ? a[0].length : 0;

    // In case it is a zero matrix, no transpose routine needed.
    if (h === 0 || w === 0) {
      return [];
    }

    let t: number[][];
    t = [];
    t.length = 0;

    // Loop through every item in the outer array (height)
    for (let i = 0; i < h; i++) {
      // Insert a new row (array)
      t.push([]);

      // Loop through every item per item in outer array (width)
      for (let j = 0; j < w; j++) {
        // Save transposed data.
        t[i][j] = a[j][i];
      }
    }

    return t;
  };

  async getDatapoints(from: string, to: string, queries: JSONQuery[]) {
    const datapoints: number[][][] = [];
    const columnNames: string[][] = [];

    for (let i = 0; i < queries.length; i += 1) {
      let parameterString = queries[i].parameterString; // This is either a string with parameters or  ''
      let location = this.template_location;
      let asset = this.template_asset;
      let value = this.template_value;
      // Define location
      if (this.isEmptyOrUndefined(location)) {
        location = queries[i].location.label;
        if (this.isEmptyOrUndefined(location)) {
          continue;
        }
      }
      // Define asset
      if (this.isEmptyOrUndefined(asset)) {
        asset = queries[i].asset.label;
        if (this.isEmptyOrUndefined(asset)) {
          continue;
        }
      }
      // Define value
      if (this.isEmptyOrUndefined(value)) {
        value = queries[i].value.label;
        if (this.isEmptyOrUndefined(value)) {
          continue;
        }
      }
      await this.backendSrv
        .datasourceRequest({
          url: this.baseURL + `/${location}/${asset}/${value}?from=${from}&to=${to}${parameterString}`,
          method: 'GET',
        })
        .then((res: any) => {
          // Handle empty responses
          if (res.data.datapoints === null) {
            return;
          }

          // Push datapoints
          columnNames.push(res.data.columnNames);
          datapoints.push(this.transpose(res.data.datapoints));
        });
    }

    return { datapoints: datapoints, columnNames: columnNames };
  }
}
