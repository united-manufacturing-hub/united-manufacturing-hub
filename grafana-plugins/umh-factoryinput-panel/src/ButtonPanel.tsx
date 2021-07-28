import React, { PureComponent } from 'react';
import { Button, IconName, ButtonVariant } from '@grafana/ui';
import { PanelProps } from '@grafana/data';
import { getTemplateSrv } from '@grafana/runtime';
import { ButtonPanelOptions, ButtonPanelState } from 'types';

interface Props extends PanelProps<ButtonPanelOptions> {}

export class ButtonPanel extends PureComponent<Props, ButtonPanelState> {
  constructor(props: any) {
    super(props);
    this.state = {
      api_call: 'READY',
      response: '',
    };
  }

  resetState() {
    this.setState({
      api_call: 'READY',
      response: '',
    });
  }

  apiStateIcon(): IconName | undefined {
    switch (this.state.api_call) {
      case 'IN_PROGRESS':
        return 'fa fa-spinner';
      case 'SUCCESS':
        return 'check';
      case 'ERROR':
        return 'exclamation-triangle';
      case 'READY':
      default:
        return this.props.options.icon;
    }
  }

  apiStateClassName() {
    switch (this.state.api_call) {
      case 'IN_PROGRESS':
        return 'spinning';
      default:
        return '';
    }
  }

  getOrientation() {
    if (!this.props.options.orientation) {
      return 'center';
    }
    switch (this.props.options.orientation) {
      case 'left':
        return 'left';
      case 'right':
        return 'right';
      case 'center':
      default:
        return 'center';
    }
  }

  customStyle() {
    if (this.props.options.variant === 'custom') {
      return {
        // Resaet Grafana defaults
        background: 'none',
        border: 'none',
        // Set custom styles
        backgroundColor: this.props.options.backgroundColor,
        color: this.props.options.foregroundColor,
      };
    } else {
      return {};
    }
  }

  variant(): ButtonVariant | undefined {
    if (this.props.options.variant === 'custom') {
      return undefined;
    } else {
      return this.props.options.variant as ButtonVariant;
    }
  }

  buttonText() {
    return this.interpolateVariables(this.props.options.text);
  }

  interpolateVariables(text: string) {
    return getTemplateSrv().replace(text, this.props.data.request?.scopedVars);
  }

  prepareFetchOpts(url: URL): RequestInit {
    const { options } = this.props;

    const requestHeaders: HeadersInit = new Headers();
    requestHeaders.set('Accept', 'application/json');

    let fetchOpts: RequestInit = {
      method: 'POST', // *GET, POST, PUT, DELETE, etc.
      mode: 'cors', // no-cors, *cors, same-origin
      cache: 'no-cache', // *default, no-cache, reload, force-cache, only-if-cached
      headers: requestHeaders,
      redirect: 'follow', // manual, *follow, error
      //referrerPolicy: 'no-referrer', // no-referrer, *no-referrer-when-downgrade, origin, origin-when-cross-origin, same-origin, strict-origin, strict-origin-when-cross-origin, unsafe-url
    };

    requestHeaders.set('Content-Type', 'POST');
    if (options.payload) {
      fetchOpts.body = this.interpolateVariables(options.payload);
    }


    return fetchOpts;
  }

  isString(input: any) : boolean {
    return !!(typeof input === 'string' && input);
  }

  render() {
    const { options } = this.props;

    const execute = () => {
      this.setState({api_call: 'IN_PROGRESS'});

      var pre_checks_successful = true;
      var response = '';

      if (!this.isString(options.url)) {
        this.setState({
          api_call: 'ERROR',
          response: "URL not set",
        });
        response = "URL not set";
        pre_checks_successful = false;
      }

      if (!this.isString(options.username)) {
        this.setState({
          api_call: 'ERROR',
          response: "Customer not set",
        });
        response = "Customer not set";
        pre_checks_successful = false;
      }


      if (!this.isString(options.Rlocation)) {
        this.setState({
          api_call: 'ERROR',
          response: "Location not set",
        });
        response = "Location not set";
        pre_checks_successful = false;
      }

      if (!this.isString(options.Rasset)) {
        this.setState({
          api_call: 'ERROR',
          response: "Asset not set",
        });
        response= "Asset not set";
        pre_checks_successful = false;
      }

      if (!this.isString(options.Rvalue)) {
        this.setState({
          api_call: 'ERROR',
          response: "Value not set",
        });
        response= "Value not set";
        pre_checks_successful = false;
      }

      if (pre_checks_successful) {
        var uri = this.interpolateVariables(options.url || '').replace(/\/$/, "");
        const url = new URL(`${uri}/${options.username}/${options.Rlocation}/${options.Rasset}/${options.Rvalue}`);


        let fetchOpts = this.prepareFetchOpts(url);

        fetch(url.toString(), fetchOpts)
            .then((response) => {
              if (response.type === 'opaque') {
                // CORS prevents us from knowing what's up - so be it
                this.setState({
                  api_call: 'READY',
                  response: 'CORS prevents access to the response',
                });
              } else if (response.ok) {
                this.setState({
                  api_call: 'SUCCESS',
                  response: response.statusText,
                });
                console.log('Requeste successful: ', response);
              } else {
                console.log('Requeste failed: ', response);
                throw new Error(response.status + response.statusText);
              }
            })
            .catch((e) => {
              this.setState({
                api_call: 'ERROR',
                response: e.message,
              });
              console.error('Request error: ', e);
            })
            .finally(() => {
              setTimeout(this.resetState.bind(this), 2500);
            });
      }else{
        console.error("Configuration error: ", response)
        setTimeout(this.resetState.bind(this), 2500);
      }
    }

    return (
      <div className={this.getOrientation()}>
        <Button
          variant={this.variant()}
          title={this.state.response}
          size="lg"
          className={this.apiStateClassName()}
          icon={this.apiStateIcon()}
          onClick={execute}
          style={this.customStyle()}
        >
          {this.buttonText()}
        </Button>
      </div>
    );
  }
}
