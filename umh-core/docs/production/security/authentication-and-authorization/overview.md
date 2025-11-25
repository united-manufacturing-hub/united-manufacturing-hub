# UMH Auth and Certificate System Overview

For authorization and authentication, we use a 2-layer solution. This separation-of-concerns might be untypical but in the context of our distributed, multi-tenant system it provides clear boundaries:

- **Layer 1**: Users and UMH Instances authenticate here either against the backend or a third party auth provider such as SAML, SSO or social login. Each user and each instance belongs to a company. This layer ensures that users can only send and receive messages to/from instnaces within their company.

- **Layer 2**: In addition to the cross-company protection of Layer 1, this layer provides provides fine-grained authorization. Each user and instance verifies the incoming messages before interpreting them, ensuring that the message actually comes from the sender and that the sender has the needed permissions to 