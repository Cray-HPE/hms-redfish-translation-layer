@Library('dst-shared@master') _

dockerBuildPipeline {
        githubPushRepo = "Cray-HPE/hms-redfish-translation-layer"
   repository = "cray"
   imagePrefix = "hms"
   app = "redfish-translation-service"
   name = "hms-redfish-translation-service"
   description = "Redfish Translation Service"
   dockerfile = "Dockerfile"
   slackNotification = ["#casmhms-builds", "", false, false, true, true]
   product = "csm"
}
