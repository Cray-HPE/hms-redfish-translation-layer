@Library('dst-shared@release/shasta-1.4') _

dockerBuildPipeline {
   repository = "cray"
   imagePrefix = "hms"
   app = "redfish-translation-service"
   name = "hms-redfish-translation-service"
   description = "Redfish Translation Service"
   dockerfile = "Dockerfile"
   slackNotification = ["#casmhms-builds", "", false, false, true, true]
   product = "csm"
}
