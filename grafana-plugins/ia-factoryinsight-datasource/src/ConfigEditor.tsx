import React, { ChangeEvent, PureComponent } from 'react';
import { LegacyForms } from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { JSONQueryOptions, MySecureJsonData } from './types';

const { SecretFormField, FormField } = LegacyForms;

interface Props extends DataSourcePluginOptionsEditorProps<JSONQueryOptions> {}

interface State {}

export class ConfigEditor extends PureComponent<Props, State> {
  onCustomerChange = (event: ChangeEvent<HTMLInputElement>) => {
    const { onOptionsChange, options } = this.props;
    const jsonData = {
      ...options.jsonData,
      customerId: event.target.value,
    };
    onOptionsChange({ ...options, jsonData });
  };

  onServerChange = (event: ChangeEvent<HTMLInputElement>) => {
    const { onOptionsChange, options } = this.props;
    const jsonData = {
      ...options.jsonData,
      serverURL: event.target.value,
    };
    onOptionsChange({ ...options, jsonData });
  };

  // Secure field (only sent to the backend)
  onAPIKeyChange = (event: ChangeEvent<HTMLInputElement>) => {
    const { onOptionsChange, options } = this.props;
    onOptionsChange({
      ...options,
      secureJsonData: {
        ...options.secureJsonData,
        apiKey: event.target.value,
      },
    });
  };

  onResetAPIKey = () => {
    const { onOptionsChange, options } = this.props;
    onOptionsChange({
      ...options,
      secureJsonFields: {
        ...options.secureJsonFields,
        apiKey: false,
      },
      secureJsonData: {
        ...options.secureJsonData,
        apiKey: '',
      },
    });
  };

  render() {
    const { options } = this.props;
    const { jsonData, secureJsonFields } = options;
    const secureJsonData = (options.secureJsonData || {}) as MySecureJsonData;

    return (
      <div className="gf-form-group">
        <h3 className="page-heading">Customer information</h3>
        <div className="gf-form-group">
          <div className="gf-form">
            <FormField
              label="Serber"
              labelWidth={6}
              inputWidth={20}
              onChange={this.onServerChange}
              value={jsonData.serverURL || ''}
              placeholder="Server URL"
            />
          </div>
          <div className="gf-form">
            <FormField
              label="Customer"
              labelWidth={6}
              inputWidth={20}
              onChange={this.onCustomerChange}
              value={jsonData.customerId || ''}
              placeholder="Customer ID"
            />
          </div>
          <div className="gf-form-inline">
            <div className="gf-form">
              <SecretFormField
                isConfigured={(secureJsonFields && secureJsonFields.apiKey) as boolean}
                value={secureJsonData.apiKey || ''}
                label="API Key"
                placeholder="secure json field (backend only)"
                labelWidth={6}
                inputWidth={20}
                onReset={this.onResetAPIKey}
                onChange={this.onAPIKeyChange}
              />
            </div>
          </div>
        </div>
      </div>
    );
  }
}
