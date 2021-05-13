import React, { Component } from 'react';
import { Switch } from '@grafana/ui';

type Props = {
  value: string;
  onChange: Function;
};

type State = {
  includeRunning: boolean;
};

// Sets the defaults when the component is loaded
export function aggregatedStatesDefaultString(): string {
  return '&includeRunning=false';
}

export class AggregatedStates extends Component<Props, State> {
  constructor(props: any) {
    // Initialise
    super(props);

    // Parse passed string and initialise state
    let initialState = false;
    try {
      const urlParameters = this.props.value.split('&');
      urlParameters.map(parameter => {
        const keyValuePair = parameter.split('=');
        // Look for inlude running
        if (keyValuePair[0] === 'includeRunning') {
          if (keyValuePair[1] === 'true' || keyValuePair[1] === 'True') {
            initialState = true;
          }
        }
      });
    } catch {
      console.log('Wrong aggregatedStates parameters. It must either be includeRunning=true or includeRunning=false ');
    }
    this.state = {
      includeRunning: initialState,
    };
  }

  onIncludeRunningChange = () => {
    let parameterString = `&includeRunning=${!this.state.includeRunning}`;
    this.setState({
      includeRunning: !this.state.includeRunning,
    });
    this.props.onChange(parameterString);
  };

  render() {
    return (
      <div className="gf-form">
        <label className="gf-form-label with-5 query-keyword">Include Running</label>
        <Switch css="" disabled={false} value={this.state.includeRunning} onChange={this.onIncludeRunningChange} />
      </div>
    );
  }
}
