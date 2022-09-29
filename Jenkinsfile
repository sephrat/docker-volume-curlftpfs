pipeline {
    agent any
    parameters {
        string(name:'PLUGIN_NAME', defaultValue: 'sephrat/curlftpfs', description: '')
        string(name:'PLUGIN_TAG', defaultValue:'next', description: '')
    }
    options {
        disableConcurrentBuilds()
        buildDiscarder(logRotator(numToKeepStr: '10'))
    }
    environment {
        PLUGIN_NAME="${params.PLUGIN_NAME}"
        PLUGIN_TAG="${params.PLUGIN_TAG}"
    }
    stages {
        stage ('Build') {
            steps {
              sh '''
                sudo make all PLUGIN_NAME=${PLUGIN_NAME} PLUGIN_TAG=${PLUGIN_TAG}
              '''
            }
        }
        stage ('Publish') {
            steps {
               sh 'sudo make push PLUGIN_NAME=${PLUGIN_NAME} PLUGIN_TAG=${PLUGIN_TAG}'
            }
        }
    }
}
