import React from 'react';
import { StandardEditorProps } from '@grafana/data';
import { TextPanelEditor } from 'TextPanelEditor';
import { EditorCodeType } from 'types';

import { ButtonPanelOptions } from './types';


interface Props extends StandardEditorProps<EditorCodeType, ButtonPanelOptions> {}

export const ButtonPayloadEditor: React.FC<Props> = ({ value, item, onChange, context }) => {
  return (
    <TextPanelEditor
      language={'json'}
      value={value}
      onChange={(code) => onChange(code)}
    />
  );
};
