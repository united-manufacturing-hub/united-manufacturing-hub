import { PanelPlugin, SelectableValue } from '@grafana/data';
import { getAvailableIcons } from '@grafana/ui';
import { ButtonPanelOptions } from './types';
import { ButtonPanel } from './ButtonPanel';
import { ButtonPayloadEditor } from './ButtonPayloadEditor';

import 'static/button-panel.css';

export const plugin = new PanelPlugin<ButtonPanelOptions>(ButtonPanel).setPanelOptions((builder) => {
  return builder
    .addTextInput({
      path: 'url',
      name: 'URL',
      category: ['REST Integration'],
      description: 'URL of the Grafana Auth proxy',
      defaultValue: 'http://grafana-proxy/',
    })
      .addTextInput({
        path: 'Rlocation',
        name: 'Location',
        category: ['REST Integration'],
        description: 'Location',
      })
      .addTextInput({
        path: 'Rasset',
        name: 'Asset',
        category: ['REST Integration'],
        description: 'Asset',
      })
      .addTextInput({
        path: 'Rvalue',
        name: 'Value',
        category: ['REST Integration'],
        description: 'Value',
      })
    .addCustomEditor({
      id: 'payload',
      path: 'payload',
      name: 'Payload',
      category: ['REST Integration'],
      description: 'MQTT payload to send',
      settings: {
        language: 'application/json',
      },
      editor: ButtonPayloadEditor,
    })
    .addSelect({
      path: 'variant',
      name: 'Variant',
      description: 'Button variant used to render',
      settings: {
        options: [
          {
            value: 'primary',
            label: 'Primary',
          },
          {
            value: 'secondary',
            label: 'Secondary',
          },
          {
            value: 'destructive',
            label: 'Destructive',
          },
          {
            value: 'link',
            label: 'Link',
          },
          {
            value: 'custom',
            label: 'Custom',
          },
        ],
      },
      defaultValue: 'primary',
    })
    .addColorPicker({
      path: 'foregroundColor',
      name: 'Foreground Color',
      description: 'Foreground color of the button',
      settings: {
        disableNamedColors: true,
      },
      showIf: (config) => config.variant === 'custom',
    })
    .addColorPicker({
      path: 'backgroundColor',
      name: 'Background Color',
      description: 'Background color of the button',
      settings: {
        disableNamedColors: true,
      },
      showIf: (config) => config.variant === 'custom',
    })
    .addRadio({
      path: 'orientation',
      name: 'Orientation',
      description: 'Button orientation used to render',
      settings: {
        options: [
          {
            value: 'left',
            label: 'Left',
          },
          {
            value: 'center',
            label: 'Center',
          },
          {
            value: 'right',
            label: 'Right',
          },
        ],
      },
      defaultValue: 'center',
    })
    .addSelect({
      path: 'icon',
      name: 'Icon',
      description: '',
      settings: {
        options: getAvailableIcons().map(
          (icon): SelectableValue => {
            return {
              value: icon,
              label: icon,
            };
          }
        ),
      },
      defaultValue: 'cog',
    })
    .addTextInput({
      path: 'text',
      name: 'Text',
      description: 'The description of the button',
      defaultValue: 'The default button label',
    });
});
