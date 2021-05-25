import React, { FormEvent, ChangeEvent, PureComponent } from 'react';
import { TextArea, InlineField, InlineFieldRow, Switch, Input } from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { MyDataSourceOptions, MySecureJsonData } from './types';

interface Props extends DataSourcePluginOptionsEditorProps<MyDataSourceOptions> {}

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

    onBrokerURIChange = (event: ChangeEvent<HTMLInputElement>) => {
        const { onOptionsChange, options } = this.props;
        const jsonData = {
            ...options.jsonData,
            brokerURI: event.target.value,
        };
        onOptionsChange({ ...options, jsonData });
    };

    onHistoricalEnabledChange = (event: ChangeEvent<HTMLInputElement>) => {
        const { onOptionsChange, options } = this.props;
        const jsonData = {
            ...options.jsonData,
            historicalEnabled: !options.jsonData.historicalEnabled,
        };
        onOptionsChange({ ...options, jsonData });
    };

    onRealtimeEnabledChange = (event: ChangeEvent<HTMLInputElement>) => {
        const { onOptionsChange, options } = this.props;
        const jsonData = {
            ...options.jsonData,
            realtimeEnabled: !options.jsonData.realtimeEnabled,
        };
        onOptionsChange({ ...options, jsonData });
    };

    onSecureConnectionMQTTChange = (event: ChangeEvent<HTMLInputElement>) => {
        const { onOptionsChange, options } = this.props;
        const jsonData = {
            ...options.jsonData,
            secureConnectionMQTT: !options.jsonData.secureConnectionMQTT,
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

    // Secure field (only sent to the backend)
    onCACertChange = (event: FormEvent<HTMLTextAreaElement>) => {
        const { onOptionsChange, options } = this.props;
        const value = event.target as HTMLTextAreaElement
        onOptionsChange({
            ...options,
            secureJsonData: {
                ...options.secureJsonData,
                CACert: value.value,
            },
        });
    };

    // Secure field (only sent to the backend)
    onPrivkeyChange = (event: FormEvent<HTMLTextAreaElement>) => {
        const { onOptionsChange, options } = this.props;
        const value = event.target as HTMLTextAreaElement
        onOptionsChange({
            ...options,
            secureJsonData: {
                ...options.secureJsonData,
                privkey: value.value,
            },
        });
    };

    render() {
        const { options } = this.props;
        const { jsonData } = options;
        const secureJsonData = (options.secureJsonData || {}) as MySecureJsonData;

        return (
            <div>
                <div className="gf-form-group">
                    <h3 className="page-heading">Historical data</h3>
                    <InlineFieldRow>
                        <InlineField label="Enabled" labelWidth={20}>
                            <Switch 
                                css="" 
                                width={40} 
                                value={jsonData.historicalEnabled} 
                                onChange={this.onHistoricalEnabledChange} 
                            />
                        </InlineField>
                    </InlineFieldRow>
                    <InlineFieldRow style={{ display: jsonData.historicalEnabled ? "block" : "none" }}>
                        <InlineField 
                            label="API path" 
                            labelWidth={20}
                            tooltip="The URL to the factoryinsight backend including a leading http or https."
                        >
                            <Input 
                                css=""
                                width={40} 
                                onChange={this.onServerChange}
                                value={jsonData.serverURL || ''}
                                placeholder="http://factoryinsight.factoryinsight"
                            />
                        </InlineField>
                    </InlineFieldRow>
                    <InlineFieldRow style={{ display: jsonData.historicalEnabled ? "block" : "none" }}>
                        <InlineField 
                            label="API token" 
                            labelWidth={20}
                            tooltip="The API token for the backend. This can be created using Basic Auth e.g., in Postman"
                        >
                            <Input 
                                css=""
                                width={40} 
                                onChange={this.onAPIKeyChange}
                                value={secureJsonData.apiKey || ''}
                                placeholder="Basic xxxxxxxx"
                            />
                        </InlineField>
                    </InlineFieldRow>
                </div>
                <div className="gf-form-group">
                    <h3 className="page-heading">Real-time data</h3>
                    <InlineFieldRow>
                        <InlineField label="Enabled" labelWidth={20}>
                            <Switch 
                                css="" 
                                width={40} 
                                value={jsonData.realtimeEnabled} 
                                onChange={this.onRealtimeEnabledChange} 
                            />
                        </InlineField>
                    </InlineFieldRow>
                    <InlineFieldRow style={{ display: jsonData.realtimeEnabled ? "block" : "none" }}>
                        <InlineField 
                            label="Broker URI" 
                            labelWidth={20}
                            tooltip="The URI to access the MQTT broker including a leading mqtt:// (unencrypted) or mqtts:// (encrypted)"
                        >
                            <Input 
                                css=""
                                width={40} 
                                onChange={this.onBrokerURIChange}
                                value={jsonData.brokerURI || ''}
                                placeholder="mqtts://vernemq.vernemq"
                            />
                        </InlineField>
                    </InlineFieldRow>
                    <InlineFieldRow style={{ display: jsonData.realtimeEnabled ? "block" : "none" }}>
                        <InlineField label="Secure connection" labelWidth={20}>
                            <Switch 
                                css="" 
                                width={40} 
                                value={jsonData.secureConnectionMQTT} 
                                onChange={this.onSecureConnectionMQTTChange} 
                            />
                        </InlineField>
                    </InlineFieldRow>
                    <InlineFieldRow style={{ display: (jsonData.realtimeEnabled && jsonData.secureConnectionMQTT) ? "block" : "none" }}>
                        <InlineField label="CA cert (PEM format)" labelWidth={20}>
                            <TextArea 
                                css=""
                                width={80} 
                                onChange={this.onCACertChange}
                                value={secureJsonData.CACert || ''}
                                placeholder="123"
                            />
                        </InlineField>
                    </InlineFieldRow>
                    <InlineFieldRow style={{ display: (jsonData.realtimeEnabled && jsonData.secureConnectionMQTT) ? "block" : "none" }}>
                        <InlineField label="Privkey (PEM format)" labelWidth={20}>
                            <TextArea 
                                css=""
                                width={80} 
                                onChange={this.onPrivkeyChange}
                                value={secureJsonData.privkey || ''}
                                placeholder="123"
                            />
                        </InlineField>
                    </InlineFieldRow>
                </div>
            </div>
        );
    }
}
