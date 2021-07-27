module github.com/Cray-HPE/hms-redfish-translation-service

go 1.16

require (
	cloud.google.com/go v0.64.0
	github.com/Cray-HPE/hms-compcredentials v1.11.2
	github.com/Cray-HPE/hms-go-http-lib v1.5.3
	github.com/Cray-HPE/hms-securestorage v1.12.2
	github.com/Cray-HPE/hms-smd v1.30.9
	github.com/go-redis/redis v6.15.2+incompatible
	github.com/google/uuid v1.1.1
	github.com/hashicorp/go-retryablehttp v0.7.0
	github.com/hashicorp/go-version v1.1.0
	github.com/mitchellh/mapstructure v1.3.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.5.1
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415
	golang.org/x/crypto v0.0.0-20200709230013-948cd5f35899
	google.golang.org/api v0.30.0
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	stash.us.cray.com/HMS/hms-base v1.13.0
)
