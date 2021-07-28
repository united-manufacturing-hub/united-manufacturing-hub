import { ButtonVariant, IconName } from '@grafana/ui';

export type EditorCodeType = string | undefined;

export interface ButtonPanelOptions {
  url: string;

  type: string;
  payload?: string;

  username?: string;

  text: string;
  variant: ButtonVariant | 'custom';
  foregroundColor?: string;
  backgroundColor?: string;
  icon?: IconName;
  orientation: string;

  //R prefixed to prevent overwriting of default keys
  Rlocation: string,
  Rasset: string,
  Rvalue: string,
}

export type ButtonPanelState = {
  api_call: 'READY' | 'IN_PROGRESS' | 'SUCCESS' | 'ERROR';
  response: string;
};


