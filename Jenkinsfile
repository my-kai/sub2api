pipeline {
    agent any

    // The image tag and remote directory are fixed deployment contracts.
    environment {
        REGISTRY_HOST = 'registry.cn-shenzhen.aliyuncs.com'
        IMAGE = 'registry.cn-shenzhen.aliyuncs.com/wdaglb/sub2api-ex:1.0'
        DEPLOY_DIR = '/www/sites/api.methink.cc/sub2api2'
    }

    parameters {
        // Jenkins global configuration differs between installations, so IDs are explicit inputs.
        string(
            name: 'REGISTRY_CREDENTIALS_ID',
            defaultValue: '',
            trim: true,
            description: 'Jenkins Username/Password credential ID for the Aliyun container registry'
        )
        string(
            name: 'SSH_SERVER_NAME',
            defaultValue: '',
            trim: true,
            description: 'Publish Over SSH server name for the production host'
        )
    }

    options {
        // Prevent two builds from replacing the same production container concurrently.
        disableConcurrentBuilds()
        timestamps()
    }

    stages {
        stage('Validate deployment inputs') {
            steps {
                script {
                    if (!params.REGISTRY_CREDENTIALS_ID?.trim()) {
                        error('REGISTRY_CREDENTIALS_ID must be provided')
                    }
                    if (!params.SSH_SERVER_NAME?.trim()) {
                        error('SSH_SERVER_NAME must be provided')
                    }
                }
            }
        }

        stage('Build and push image') {
            steps {
                script {
                    withCredentials([
                        usernamePassword(
                            credentialsId: params.REGISTRY_CREDENTIALS_ID,
                            usernameVariable: 'REGISTRY_USERNAME',
                            passwordVariable: 'REGISTRY_PASSWORD'
                        )
                    ]) {
                        sh '''
                            set -eu

                            # Login only for this build so registry credentials are not persisted in the workspace.
                            printf '%s' "$REGISTRY_PASSWORD" | docker login "$REGISTRY_HOST" \
                                --username "$REGISTRY_USERNAME" \
                                --password-stdin
                            trap 'docker logout "$REGISTRY_HOST" >/dev/null 2>&1 || true' EXIT

                            docker build --pull --tag "$IMAGE" .
                            docker push "$IMAGE"
                        '''
                    }
                }
            }
        }

        stage('Deploy through Publish Over SSH') {
            steps {
                script {
                    // Build the remote script without interpolating its shell variables in Groovy.
                    // The fixed values are passed as environment variables to keep the command auditable.
                    def remoteCommand = "IMAGE='${env.IMAGE}' DEPLOY_DIR='${env.DEPLOY_DIR}' /bin/sh <<'REMOTE_SCRIPT'\n" + '''
                        set -eu

                        cd "$DEPLOY_DIR"
                        test -f .env || {
                            echo "Missing production environment file: $DEPLOY_DIR/.env" >&2
                            exit 1
                        }

                        bind_host="$(awk -F= '$1 == "BIND_HOST" { print $2; exit }' .env | tr -d '\\r')"
                        server_port="$(awk -F= '$1 == "SERVER_PORT" { print $2; exit }' .env | tr -d '\\r')"
                        if [ -z "$bind_host" ]; then bind_host='0.0.0.0'; fi
                        if [ -z "$server_port" ]; then server_port='8080'; fi
                        case "$server_port" in
                            *[!0-9]*|'')
                                echo "SERVER_PORT must be numeric: $server_port" >&2
                                exit 1
                                ;;
                        esac

                        # Pull before stopping the old container so a registry failure leaves production running.
                        docker pull "$IMAGE"
                        docker rm --force sub2api >/dev/null 2>&1 || true

                        mkdir -p "$DEPLOY_DIR/data"
                        docker run --detach \
                            --name sub2api \
                            --restart unless-stopped \
                            --env-file .env \
                            --publish "$bind_host:$server_port:8080" \
                            --volume "$DEPLOY_DIR/data:/app/data" \
                            "$IMAGE"

                        running_status="$(docker inspect --format '{{.State.Running}}' sub2api)"
                        test "$running_status" = 'true' || {
                            echo 'The new sub2api container did not remain running' >&2
                            docker logs --tail 100 sub2api >&2 || true
                            exit 1
                        }
                    ''' + "\nREMOTE_SCRIPT\n"

                    sshPublisher(
                        publishers: [
                            sshPublisherDesc(
                                configName: params.SSH_SERVER_NAME,
                                transfers: [
                                    sshTransfer(
                                        cleanRemote: false,
                                        excludes: '',
                                        execCommand: remoteCommand,
                                        execTimeout: 120000,
                                        flatten: false,
                                        makeEmptyDirs: false,
                                        noDefaultExcludes: false,
                                        patternSeparator: '[, ]+',
                                        remoteDirectory: '',
                                        remoteDirectorySDF: false,
                                        removePrefix: '',
                                        sourceFiles: ''
                                    )
                                ],
                                usePromotionTimestamp: false,
                                useWorkspaceInPromotion: false,
                                verbose: true
                            )
                        ]
                    )
                }
            }
        }
    }

    post {
        // Keep the final status visible in Jenkins without exposing configuration values.
        always {
            echo "Deployment pipeline finished with status: ${currentBuild.currentResult}"
        }
    }
}
