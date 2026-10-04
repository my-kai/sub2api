pipeline {
    agent any

    // 镜像标签和远程目录是固定的部署约定。
    environment {
        REGISTRY_HOST = 'registry.cn-shenzhen.aliyuncs.com'
        IMAGE = 'registry.cn-shenzhen.aliyuncs.com/wdaglb/sub2api-ex:1.0'
        DEPLOY_DIR = '/www/sites/api.methink.cc/sub2api2'
    }

    parameters {
        // 不同 Jenkins 安装的全局配置可能不同，因此将凭据 ID 和服务器名称作为显式输入。
        string(
            name: 'REGISTRY_CREDENTIALS_ID',
            defaultValue: '',
            trim: true,
            description: '阿里云容器镜像仓库的 Jenkins 用户名/密码凭据 ID'
        )
        string(
            name: 'SSH_SERVER_NAME',
            defaultValue: '',
            trim: true,
            description: '生产主机对应的 Publish Over SSH 服务器名称'
        )
    }

    options {
        // 防止两个构建任务同时替换同一个生产容器。
        disableConcurrentBuilds()
        timestamps()
    }

    stages {
        stage('验证部署参数') {
            steps {
                script {
                    if (!params.REGISTRY_CREDENTIALS_ID?.trim()) {
                        error('必须提供 REGISTRY_CREDENTIALS_ID')
                    }
                    if (!params.SSH_SERVER_NAME?.trim()) {
                        error('必须提供 SSH_SERVER_NAME')
                    }
                }
            }
        }

        stage('构建并推送镜像') {
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

                            # 仅在本次构建中登录，避免将镜像仓库凭据持久化到工作区。
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

        stage('通过 Docker Compose 部署') {
            steps {
                script {
                    // 构建远程脚本时不在 Groovy 中插入其中的 Shell 变量。
                    // 将固定值作为环境变量传入，使命令便于审计。
                    def remoteCommand = "IMAGE='${env.IMAGE}' DEPLOY_DIR='${env.DEPLOY_DIR}' /bin/sh <<'REMOTE_SCRIPT'\n" + '''
                        set -eu

                        cd "$DEPLOY_DIR"
                        test -f .env || {
                            echo "缺少生产环境文件：$DEPLOY_DIR/.env" >&2
                            exit 1
                        }

                        bind_host="$(awk -F= '$1 == "BIND_HOST" { print $2; exit }' .env | tr -d '\\r')"
                        server_port="$(awk -F= '$1 == "SERVER_PORT" { print $2; exit }' .env | tr -d '\\r')"
                        if [ -z "$bind_host" ]; then bind_host='0.0.0.0'; fi
                        if [ -z "$server_port" ]; then server_port='8080'; fi
                        case "$server_port" in
                            *[!0-9]*|'')
                                echo "SERVER_PORT 必须是数字：$server_port" >&2
                                exit 1
                                ;;
                        esac

                        # 使用已由 Jenkins 上传到远程项目目录的 Compose 模板。
                        # 使用宿主机目录挂载数据，避免切换到命名卷后丢失现有数据。
                        mkdir -p "$DEPLOY_DIR/data"
                        compose_file="$DEPLOY_DIR/docker-compose.yml"
                        test -f "$compose_file" || {
                            echo "上传的 Compose 模板不存在：$compose_file" >&2
                            exit 1
                        }

                        # 替换模板占位符后再执行 Compose，确保实际部署参数明确写入项目文件。
                        sed \
                            -e "s|__IMAGE__|$IMAGE|g" \
                            -e "s|__BIND_HOST__|$bind_host|g" \
                            -e "s|__SERVER_PORT__|$server_port|g" \
                            -e "s|__DEPLOY_DIR__|$DEPLOY_DIR|g" \
                            "$compose_file" > "$compose_file.tmp"
                        mv "$compose_file.tmp" "$compose_file"
                        if grep -qE '__[A-Z_]+__' "$compose_file"; then
                            echo 'Compose 模板占位符替换不完整' >&2
                            exit 1
                        fi

                        # 先拉取镜像再移除旧容器，确保镜像仓库故障时生产环境仍在运行。
                        docker compose -p sub2api -f "$compose_file" pull
                        # 旧容器可能由 docker run 创建，先移除同名容器以便 Compose 接管。
                        docker rm --force sub2api >/dev/null 2>&1 || true
                        docker compose -p sub2api -f "$compose_file" up -d --force-recreate

                        running_status="$(docker inspect --format '{{.State.Running}}' sub2api)"
                        test "$running_status" = 'true' || {
                            echo '新的 sub2api 容器未能保持运行' >&2
                            docker compose -p sub2api -f "$compose_file" logs --tail 100 sub2api >&2 || true
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
                                        remoteDirectory: "${env.DEPLOY_DIR}",
                                        remoteDirectorySDF: false,
                                        removePrefix: '',
                                        sourceFiles: 'docker-compose.yml'
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
        // 在 Jenkins 中显示最终状态，但不暴露配置值。
        always {
            echo "部署流水线已结束，状态：${currentBuild.currentResult}"
        }
    }
}
