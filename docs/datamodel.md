
# Introduction

This data model maps various machine states to relevant OEE buckets.

This data model is based on the following specifications:
- Weihenstephaner Standards 09.01 (for filling)
- Omron PackML (for packaging/filling)
- EUROMAP 84.1 (for plastic)
- OPC 30060 (for tobacco machines)
- VDMA 40502 (for CNC machines)

Additionally, the following literature is respected:
- Steigerung der Anlagenproduktivität durch OEE-Management (Focke, Steinbeck)

## Abbreviations 
- WS --> "TAG NAME": Valuename (number)
- PackML --> Statename (number)
- EUROMAP --> Statusname (number)
- Tobacco --> ControlModeName (number)

## ACTIVE (10000-29999)

### 10000: Producing at set speed
- WS_Cur_State: Operating 
- PackML/Tobacco: Execute 

### 20000: Producing at lower than set speed
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

## UNKNOWN

### 30000: Undefined / no data 
- WS_Cur_Prog: Undefined 
- EUROMAP: Offline

### 40000: Unspecified stop
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

### 50000: Microstop

## MATERIAL

### 60000: Jam in the inlet
- WS_Cur_State: Lack 

### 70000: Jam in the outlet
- WS_Cur_State: Tailback 

### 80000: Congestion or deficiency in the bypass flow
- WS_Cur_State: Lack/Tailback Branch Line 

### 90000: Unknown material issue
- WS_Mat_Ready (Information about which material is lacking)
- PackML/Tobacco: Suspended 

## PROCESS

### 100000: Changeover
- WS_Cur_Prog: Program-Changeover 
- Tobacco: CHANGE OVER 

### 110000: Cleaning
- WS_Cur_Prog: Program-Cleaning 
- Tobacco: CLEAN 

### 120000: Emptying
- Tobacco: EMPTY OUT 

### 130000: Setting up (warming up, etc.)
- EUROMAP: PREPARING 

## OPERATOR

### 140000: Operator not at machine (e.g. Other reasons for operator not at machine)

### 150000: Operator break (note: different than planned shift as it could count to performance losses)
- WS_Cur_Prog: Program-Break 

## PLANNING
### 160000: No shift
### 170000: No order

## TECHNICAL

### 180000: Equipment failure / repair (e.g. broken Engine)
- WS_Cur_State: Equipment Failure 

### 190000: External failure / repair (e.g. missing compressed air)
- WS_Cur_State: External Failure 

### 200000: External Interference

### 210000: Preventive Maintenance (a planned maintenance action)
- WS_Cur_Prog: Program-Maintenance 
- PackML: Maintenance 
- EUROMAP: MAINTENANCE 
- Tobacco: MAINTENANCE 

### 220000: Other technical stops
- WS_Not_Of_Fail_Code
- PackML: Held 
- EUROMAP: MALFUNCTION 
- Tobacco: MANUAL 
- Tobacco: SET UP 
- Tobacco: REMOTE SERVICE 