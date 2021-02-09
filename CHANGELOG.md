# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

<!--
Guiding Principles:
* Changelogs are for humans, not machines.
* There should be an entry for every single version.
* The same types of changes should be grouped.
* Versions and sections should be linkable.
* The latest version comes first.
* The release date of each version is displayed.
* Mention whether you follow Semantic Versioning.

Types of changes:
Added - for new features
Changed - for changes in existing functionality
Deprecated - for soon-to-be removed features
Removed - for now removed features
Fixed - for any bug fixes
Security - in case of vulnerabilities
-->

## [1.8.4] - 2021-02-08

### Changed
- Update Copyright/License info

## [1.8.3] - 2020-12-10

### Changed
- CASMHMS-4278 - Update to loftsman/docker-kubectl image to production version.

## [1.8.2] - 2020-11-16
### Security
- CASMHMS-3457 - Removed hard coded passwords


## [1.8.1] - 2020-10-21
### Security
- CASMHMS-4105 - Updated base Golang Alpine image to resolve libcrypto vulnerability.

## [1.8.0] - 2020-10-01
### Added 
- CASMHMS-4020 - Added the Redfish Certificate Service to RTS, and supporting manager resources. It is enabled for the JAWS, GCloud, RF Simulator Backend helpers. This new services allows for the replacement of the HTTPS certificate that is used during HTTPS connections.

### Changed
- Altered the rfdispatcher to support multiple backend helpers. The Certificate service was implemented as a core backend helper that always runs first, then the device specific backend helper runs (JAWS, GCloud, RF Simulator).
- The Redfish Schema tree generation logic was updated to support navigation links that are nested in top level object properties. 

## [1.7.1] - 2020-09-29
### Changed
- CASMHMS-3822 - Use k8s services to provide "DNS" entries for each PDU exposed by RTS, this is the same mechanism that the GCloud backend helper uses for Virtual Shasta.  This allow any service within the service mesh to communicate with PDUs via RTS. 
- CASMHMS-4039 - When initializing a PDU disable the static IP fallback feature. This will prevent the PDU falling back to an static address when DHCP is not available.

## [1.7.0] - 2020-09-17
### Added 
- CASMHMS-4008 - Added PDU temperature and humidity sensors support to the JAWS backend helper under `/redfish/v1/PowerEquipment/RackPDUs/{pdu_id}/Sensors/{sensor_id}`.
- CASMHMS-4008 - Added streaming telemetry for Temperature and Humidity sensors. 

### Security
- CASMHMS-3995 - Updated RTS to use latest trusted baseOS images.

## [1.6.0] - 2020-09-15

## Changed
- CASMCLOUD-1023 - Updated cray-service base chart to the latest 2.x version. This new version of the cray service chart now supports Helm v3. 
- Modified containers/init containers, volume, and persistent volume claim value definitions to be objects instead of arrays

## [1.5.0] - 2020-09-10
### Added
- CASMHMS-3967 - Added Streaming telemetry for PDUs in the JAWs backend helper.

### Changed
- PDU outlets exposed by Redfish now adds padding to outlet numbers, outlet `BA1` now becomes `BA01` for example.  This allows SMD to assign the correct PDU Power connector xnames to each outlet, as SMD sorts the PDU outlet members lexicographically to determine the number of the outlet.  

## [1.4.1] - 2020-09-01
### Added
- CASMHMS-3966 - Added support in the JAWS Backend Helper to poll PDUs. 

## [1.4.0] - 2020-08-20
### Changed
- CASMHMS-3827 - Improved Handling of credentials in the JAWS Backend Helper. The Vault Loader will now loads the default credentials for PDUs and RTS redfish interface. 

## [1.3.1] - 2020-07-22

### Changed

CASMHMS-3418 - Updated Docker builds to use latest trusted base images.

## [1.3.0] - 2020-07-14

### FIXED

CASMHMS-3721: OData ID and OData Type annotations in annotations now have the correct values, instead of null.
CASMHMS-3732: Fixed an issue with the GCloud where it would return a 500 status code, when performing a GET request on `/redfish/v1/Systems/Self`

## [1.2.3] - 2020-06-08

### Changed

SKERN-2849 - Removed filter from gcloud backend_helper xname discovery

## [1.2.2] - 2020-05-08

### Changed

CASMHMS-3264 - updated cray-service base chart to enable online install upgrade/rollback

## [1.2.1] - 2020-05-07

### Changed

- CASMHMS-2963 - Use trusted baseOS images for docker builds.

## [1.2.0] - 2020-04-16

### Added

- Added simulated composed environment.
