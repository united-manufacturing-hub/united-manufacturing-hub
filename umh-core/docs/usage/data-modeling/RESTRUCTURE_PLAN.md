# Documentation Restructure Plan for ENG-3100

## Overview
The ENG-3100 issue introduces a fundamental restructuring of the YAML configuration format. This plan addresses the complete transformation from the current structure to the proposed format.

## Core Structure Changes

### 1. **data-models.md** - Major Restructuring

#### A. Add New Payload Shapes Section (at beginning of file)
```yaml
# New section to add
payloadshapes:
  timeseries-number:
    fields:
      timestamp_ms:
        _type: number
      value:
        _type: number
  timeseries-string:
    fields:
      timestamp_ms:
        _type: number
      value:
        _type: string
  relational-employee: # Not to be implemented in MVP
    fields:
      employee_id:
        _type: string
      first_name:
        _type: string
      last_name:
        _type: string
      department:
        _type: string
      health_metrics:
        pulse:
          value:
            _type: number
          measured_at:
            _type: number
```

#### B. Replace Current Data Models Structure
**Current:**
```yaml
datamodels:
  - name: ModelName
    version: v1
    structure:
      field:
        type: timeseries
```

**New:**
```yaml
datamodels:
  pump:
    description: "pump from vendor ABC"
    versions:
      v1:
        root:
          count:
            _payloadshape: timeseries-number
          serialNumber:
            _type: timeseries-string
```

#### C. Update All Field Syntax
- Replace `type: timeseries` with `_payloadshape: timeseries-number`
- Add payload shape references: `_payloadshape: timeseries-string`
- Add direct type references where appropriate: `_type: timeseries-string`
- Note: `_type:` can reference both basic types (`number`, `string`) and payload shapes

#### D. Update Sub-Model References
**Current:**
```yaml
motor:
  _model: Motor:v1
```

**New:**
```yaml
motor:
  _refModel: 
    name: motor
    version: v1
```

#### E. Add Roadmap Items
Add roadmap markers for Post-MVP features:
```yaml
z-axis:
  _payloadshape: timeseries-number
  _meta: # Not to be implemented in MVP
    description: "ABC"
    unit: "m/s"
  _constraints: # Not to be implemented in MVP
    max: 100
    min: 0
```

### 2. **data-contracts.md** - Align with New Structure

#### A. Update Model References
- Change contract model references to match new naming
- Update all example YAML to use new structure

#### B. Update Generated Schema Examples
- Modify TimescaleDB schema examples to reflect new field naming
- Update JSON location structure examples

#### C. Update Cross-References
- Ensure all references to data models use new syntax
- Update links to payload shapes section

### 3. **stream-processors.md** - Minimal Changes

#### A. Update Examples
- Modify YAML examples to reference new data model structure
- Ensure contract references align with new format

#### B. Update Generated Topics
- Verify topic examples match new model structure
- Update any field path references

## Post-MVP: Content Enhancement

### 4. **Enhanced Payload Shapes Documentation**

#### A. Add Comprehensive Examples
```yaml
payloadshapes:
  relational-employee: # Not to be implemented in MVP
    description: "Employee relational data structure"
    fields:
      employee_id:
        _type: string
      first_name:
        _type: string
      last_name:
        _type: string
      department:
        _type: string
      health_metrics:
        pulse:
          value:
            _type: number
          measured_at:
            _type: number
```

#### B. Add Usage Guidelines
- When to create custom payload shapes
- Best practices for shape design
- Reusability considerations

### 5. **Advanced Data Model Features**

#### A. Complex Nested Structures
Update examples to show:
- Deep folder hierarchies
- Complex sub-model composition
- Mixed payload shape usage

#### B. Versioning Strategy
- Document nested version management
- Show evolution examples
- Explain backward compatibility

## Validation and Cross-References

### 6. **Consistency Validation**

#### A. Cross-File Consistency
- Ensure all examples use same model names
- Verify all payload shape references exist
- Check all sub-model references are valid

#### B. Link Validation
- Update all internal links
- Verify external references
- Check anchor links

### 7. **Example Standardization**

#### A. Consistent Examples
Use these standard examples across all files:
- `motor` model with `current`, `rpm`, `temperature`
- `pump` model with `motor` sub-model
- `employee` relational model for complex data (Post-MVP scope - not to be implemented in MVP)

#### B. Progressive Complexity
- Start with simple examples
- Build to complex nested structures
- Show real-world use cases

## Implementation Sequence

### Step 1: Foundation (data-models.md)
1. Add payload shapes section
2. Update basic data model structure
3. Update field syntax throughout
4. Add roadmap markers

### Step 2: Contracts (data-contracts.md)
1. Update model references
2. Align examples with new structure
3. Update generated schema examples

### Step 3: Processors (stream-processors.md)
1. Update examples to match new structure
2. Verify topic paths align
3. Update cross-references

### Step 4: Validation
1. Check all cross-references
2. Validate all YAML examples
3. Ensure consistent terminology

## Key Considerations

### Critical Format Changes
- **Payload Shapes**: Use object keys directly (`timeseries-number:`) not arrays (`- name: timeseries`)
- **No Versioning**: Payload shapes do not include version fields (unlike current docs)
- **Naming Convention**: Use hyphenated names (`timeseries-number`, `timeseries-string`) 
- **Model Names**: Use lowercase names (`pump`, `motor`) not capitalized (`Pump`, `Motor`)
- **Field Types**: `_type:` can reference both basic types AND payload shapes

### Backward Compatibility Notes
- Document that this is a breaking change
- Provide migration guide if needed
- Explain timeline for implementation

### Roadmap Items
Mark these features as future enhancements (Post-MVP):
- `_meta` fields for descriptions and units (marked as "Not to be implemented in MVP")
- `_constraints` for validation rules (marked as "Not to be implemented in MVP")
- Advanced relational modeling features (marked as "Not to be implemented in MVP")

### Documentation Quality
- Maintain existing tone and style
- Keep examples practical and relevant
- Ensure clear progression from simple to complex

## Detailed Changes by File

### data-models.md Changes
1. **Add Payload Shapes section** (new, at top)
2. **Replace "Structure Elements" section** with new syntax
3. **Update all YAML examples** throughout
4. **Add roadmap callouts** for _meta and _constraints
5. **Update cross-references** to other files

### data-contracts.md Changes
1. **Update model binding examples** to new format
2. **Modify generated schema examples** 
3. **Update validation examples**
4. **Fix cross-references** to data models

### stream-processors.md Changes
1. **Update contract binding examples**
2. **Modify mapping examples** if needed
3. **Update generated topic examples**
4. **Fix cross-references**

## Success Criteria
- [ ] All YAML examples use new ENG-3100 syntax
- [ ] Payload shapes section is comprehensive
- [ ] Cross-references between files are consistent
- [ ] Roadmap items are clearly marked
- [ ] Examples progress logically from simple to complex
- [ ] Documentation maintains existing quality and tone 