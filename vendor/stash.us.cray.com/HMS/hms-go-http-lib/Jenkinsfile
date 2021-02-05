@Library('dst-shared@master') _

dockerBuildPipeline {
        githubPushRepo = "Cray-HPE/hms-go-http-lib"
        repository = "cray"
        imagePrefix = "hms"
        app = "hms-go-http-lib"
        name = "hms-go-http-lib"
        description = "Cray HMS http library package."
        dockerfile = "Dockerfile"
        slackNotification = ["", "", false, false, true, true]
        product = "internal"
}
