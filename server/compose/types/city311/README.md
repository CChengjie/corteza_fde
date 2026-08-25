# City 311 shared contract

This package is the Developer 1-owned contract boundary for the City 311 adaptation. It freezes consumer-visible field names and schemas, controlled vocabularies, lifecycle transitions, shared browser protocol mechanics, endpoint methods, paths and direction, authentication and record scope, response statuses and headers, and deterministic mock responses that Developer 2 can use before the backing services are complete.

`contract.json` is the generated language-neutral frontend handoff. `NewContractDocument` is its complete Go source. The contract test compares the entire semantic document in both directions, including key sets, schemas, endpoint behavior, headers, provisions, mocks, and versioning metadata.

The exact leaf provisions implemented or verified are recorded in `contract.json`; section-level ranges are intentionally not used. The contract covers shared session, authorization, optimistic-concurrency, validation, list, idempotency, asynchronous-operation, bulk-failure, geocoding and attachment conventions together with the public portal, staff request handling, administration, reporting, mail, calendar and external-integration surfaces. Localised display strings remain in Developer 2's translation catalogue and are not duplicated here.

The contract records explicit integration decisions where the specification fixes behavior but not internal routes or representation details. Notably, CivicWorks direct completion is normalised atomically through the legal CRM lifecycle, terminal redelivery is acknowledged idempotently, portal attachments use staged uploads while the integration API retains inline base64, anonymous lookup uses a privacy-safe projection, and application roles are kept distinct from identity-provider and audit actor vocabularies. This package defines the contract only; runtime routes and persistence implement it elsewhere.

Developer 1 is the designated maintainer for this package and its generated or shared contract artifacts.
