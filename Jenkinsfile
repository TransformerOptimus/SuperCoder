pipeline {
    agent {
        kubernetes {
            inheritFrom 'default'
            defaultContainer 'docker'
        }
    }
    stages {
        stage('Build & Push') {
            steps {
                script {
                    dockerBuildX(
                            repository = "ai-developer-api",
                            customArgs = " --target production ",
                            path = ".",
                            dockerFile = "Dockerfile",
                            accountId = "767397669303",
                            region = "us-west-2",
                    )
                    dockerBuildX(
                            repository = "ai-developer-migrations",
                            customArgs = " --target migrations ",
                            path = ".",
                            dockerFile = "Dockerfile",
                            accountId = "767397669303",
                            region = "us-west-2",
                    )
                    dockerBuildX(
                            repository = "ai-developer-worker",
                            customArgs = " --target production ",
                            path = ".",
                            dockerFile = "Dockerfile",
                            accountId = "767397669303",
                            region = "us-west-2",
                    )
                    dockerBuildX(
                            repository = "ai-developer-python-executor",
                            customArgs = " --target python-executor ",
                            path = ".",
                            dockerFile = "Dockerfile",
                            accountId = "767397669303",
                            region = "us-west-2",
                    )
                    dockerBuildX(
                            repository = "ai-developer-node-executor",
                            customArgs = " --target node-executor ",
                            path = ".",
                            dockerFile = "Dockerfile",
                            accountId = "767397669303",
                            region = "us-west-2",
                    )
                }
            }
        }

        stage('Update Tag') {
            steps {
                updateTag("supercoder/backend-api", "${env.GIT_COMMIT}")
                updateTag("supercoder/backend-worker", "${env.GIT_COMMIT}")
            }
        }
    }
}
