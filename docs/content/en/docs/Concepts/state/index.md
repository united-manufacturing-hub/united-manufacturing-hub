
---
title: "Available states for assets"
linkTitle: "Available states for assets"
weight: 3
description: >
  This data model maps various machine states to relevant OEE buckets.
---

## Introduction

This data model is based on the following specifications:

- Weihenstephaner Standards 09.01 (for filling)
- Omron PackML (for packaging/filling)
- EUROMAP 84.1 (for plastic)
- OPC 30060 (for tobacco machines)
- VDMA 40502 (for CNC machines)

Additionally, the following literature is respected:

- Steigerung der Anlagenproduktivität durch OEE-Management (Focke, Steinbeck)

### Abbreviations

- WS --> "TAG NAME": Valuename (number)
- PackML --> Statename (number)
- EUROMAP --> Statusname (number)
- Tobacco --> ControlModeName (number)

## ACTIVE (10000-29999)

The asset is actively producing.

### 10000: ProducingAtFullSpeedState

The asset is running on full speed.

#### Examples for ProducingAtFullSpeedState

- WS_Cur_State: Operating
- PackML/Tobacco: Execute

### 20000: ProducingAtLowerThanFullSpeedState

The asset is NOT running on full speed.

#### Examples for ProducingAtLowerThanFullSpeedState

- WS_Cur_Prog: StartUp
- WS_Cur_Prog: RunDown
- WS_Cur_State: Stopping
- PackML/Tobacco: Stopping
- WS_Cur_State: Aborting
- PackML/Tobacco: Aborting
- WS_Cur_State: Holding
- WS_Cur_State: Unholding
- PackML/Tobacco: Unholding
- WS_Cur_State: Suspending
- PackML/Tobacco: Suspending
- WS_Cur_State: Unsuspending
- PackML/Tobacco: Unsuspending
- PackML/Tobacco: Completing
- WS_Cur_Prog: Production
- EUROMAP: MANUAL_RUN
- EUROMAP: CONTROLLED_RUN
  
NOT INCLUDED FOR NOW:

- WS_Prog_Step: all

## UNKNOWN (30000-59999)

The asset is in an unspecified state.

### 30000: UnknownState

We do not have any data for that asset (e.g. connection to PLC aborted).

#### Examples for UnknownState

- WS_Cur_Prog: Undefined
- EUROMAP: Offline

### 40000: UnspecifiedStopState

The asset is not producing, but we do not know why (yet).

#### Examples for UnspecifiedStopState

- WS_Cur_State: Clearing
- PackML/Tobacco: Clearing
- WS_Cur_State: Emergency Stop
- WS_Cur_State: Resetting
- PackML/Tobacco: Clearing
- WS_Cur_State: Held
- EUROMAP: Idle
- Tobacco: Other
- WS_Cur_State: Stopped
- PackML/Tobacco: Stopped
- WS_Cur_State: Starting
- PackML/Tobacco: Starting
- WS_Cur_State: Prepared
- WS_Cur_State: Idle
- PackML/Tobacco: Idle
- PackML/Tobacco: Complete
- EUROMAP: READY_TO_RUN

### 50000: MicrostopState

The asset is not producing for a short period (typically around 5 minutes), but we do not know why (yet).

## MATERIAL (60000-99999)

The asset has issues with materials.

### 60000: InletJamState

The machine does not perform its intended function due to a lack of material flow in the infeed of the machine detected by the sensor system of the control system (machine stop). In the case of machines that have several inlets, the condition of lack in the inlet refers to the main flow, i.e. to the material (crate, bottle) that is fed in the direction of the filling machine (central machine). The defect in the infeed is an extraneous defect, but because of its importance for visualization and technical reporting, it is recorded separately.

#### Examples for InletJamState

- WS_Cur_State: Lack

### 70000: OutletJamState

The machine does not perform its intended function as a result of a jam in the good flow discharge of the machine detected by the sensor system of the control system (machine stop). In the case of machines that have several discharges, the jam in the discharge condition refers to the main flow, i.e. to the good (crate, bottle) that is fed in the direction of the filling machine (central machine) or is fed away from the filling machine. The jam in the outfeed is an external fault 1v, but it is recorded separately "because" of its importance for visualization and technical reporting.

#### Examples for OutletJamState

- WS_Cur_State: Tailback

### 80000: CongestionBypassState

The machine does not perform its intended function due to a shortage in the bypass supply or a jam in the bypass discharge of the machine detected by the sensor system of the control system (machine stop). This condition can only occur in machines that have two outlets or inlets and in which the bypass is in turn the inlet or outlet of an upstream or downstream machine of the filling line (packaging and palletizing machines). The jam/shortage in the auxiliary flow is an external fault, but is recorded separately due to its importance for visualization and technical reporting.

#### Examples for CongestionBypassState

- WS_Cur_State: Lack/Tailback Branch Line

### 90000: MaterialIssueOtherState

The asset has a material issue, but it is not further specified.

#### Examples for MaterialIssueOtherState

- WS_Mat_Ready (Information about which material is lacking)
- PackML/Tobacco: Suspended

## PROCESS (100000-139999)

The asset is in a stop which is belongs to the process and cannot be avoided.

### 100000: ChangeoverState

The asset is in a changeover process between products.

#### Examples for ChangeoverState

- WS_Cur_Prog: Program-Changeover
- Tobacco: CHANGE OVER

### 110000: CleaningState

The asset is currently in a cleaning process.

#### Examples for CleaningState

- WS_Cur_Prog: Program-Cleaning
- Tobacco: CLEAN

### 120000: EmptyingState

The asset is currently emptied, e.g. to prevent mold for food products over the long breaks like the weekend.

#### Examples for EmptyingState

- Tobacco: EMPTY OUT

### 130000: SettingUpState

The machine is currently preparing itself for production, e.g. heating up.

#### Examples for SettingUpState

- EUROMAP: PREPARING

## OPERATOR (140000-159999)

The asset is stopped because of the operator.

### 140000: OperatorNotAtMachineState

The operator is not at the machine.

### 150000: OperatorBreakState

The operator is in a break. note: different than planned shift as it could count to performance losses

#### Examples for OperatorBreakState

- WS_Cur_Prog: Program-Break

## PLANNING (150000-179999)

The asset is stopped as it is planned to stop (planned idle time).

### 160000: NoShiftState

There is no shift planned at that asset.

### 170000: NoOrderState

There is no order planned at that asset.

## TECHNICAL (180000-229999)

The asset has a technical issue.

### 180000: EquipmentFailureState

The asset itself is defect, e.g. a broken engine.

#### Examples for EquipmentFailureState

- WS_Cur_State: Equipment Failure

### 190000: ExternalFailureState

There is a external failure, e.g. missing compressed air

#### Examples for ExternalFailureState

- WS_Cur_State: External Failure

### 200000: ExternalInterferenceState

There is an external interference, e.g. the crane to move the material is currently unavailable.

### 210000: PreventiveMaintenanceStop

A planned maintenance action.

#### Examples for PreventiveMaintenanceStop

- WS_Cur_Prog: Program-Maintenance
- PackML: Maintenance
- EUROMAP: MAINTENANCE
- Tobacco: MAINTENANCE

### 220000: TechnicalOtherStop

The asset has a technical issue, but it is not specified further.

#### Examples for TechnicalOtherStop

- WS_Not_Of_Fail_Code
- PackML: Held
- EUROMAP: MALFUNCTION
- Tobacco: MANUAL
- Tobacco: SET UP
- Tobacco: REMOTE SERVICE
