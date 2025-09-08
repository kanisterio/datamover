#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

SUPPORTED_PROTOCOL=kopia-v0-17-0

protocol=""
port=""

## TODO: do we need a list of supported protocols??
protocols_supported() {
    local protocols=$1
    for p in $(echo $protocols | tr ";" " "); do
        local p_name=$(echo $p | cut -d":" -f1)
        local p_port=$(echo $p | cut -d":" -f2)
        if [[ $p_name == $SUPPORTED_PROTOCOL ]]; then
            protocol=$p_name
            port=$p_port
            return 0
        fi
    done
    return 1
}

protocols_supported ${PROTOCOLS:?"PROTOCOLS must be defined"}

echo "Using protocol ${protocol} with PORT=${port}"

## Read server config

## FIXME: validate all configs and fail on invalid

config_dir=/etc/config
## FIXME: what to do if the file is not there?
## Some files are mandatory, others might be optional?
hostname=$(cat ${config_dir}/hostname)
username=$(cat ${config_dir}/username)
## FIXME: figure out rootPath VS storage_prefix
rootPath=$(cat ${config_dir}/rootPath)

cacheDirectory=$(cat ${config_dir}/cacheDirectory)
configFilePath=$(cat ${config_dir}/configFilePath)
logDirectory=$(cat ${config_dir}/logDirectory)
contentCacheSizeMb=$(cat ${config_dir}/contentCacheSizeMb)
metadataCacheSizeMb=$(cat ${config_dir}/metadataCacheSizeMb)
read_only=$(cat ${config_dir}/readOnly || echo "false")

## Read secret configurations
secrets_dir=/etc/secrets
repo_secret=${secrets_dir}/repo-access
repo_password=$(cat ${repo_secret}/password)

admin_secret=${secrets_dir}/admin
admin_password=$(cat ${admin_secret}/password)
admin_username=$(cat ${admin_secret}/username)

storage_secret=${secrets_dir}/storage
storage_type=$(cat ${storage_secret}/type)

storage_creds_secret=${secrets_dir}/storage-credentials

## FIXME: optional secret
tls_cert_secret=${secrets_dir}/tls-cert
tls_cert_file=${tls_cert_secret}/tls.crt

tls_key_secret=${secrets_dir}/tls-key
tls_key_file=${tls_key_secret}/tls.key

## Env variables controlling operation
## CREATE_REPO_IF_NOT_EXIST - create repo if it doesn't exist
## false | true

## Env variables for kopia config
## KOPIA_ALLOW_WRITE_ON_INDEX_LOAD
## GetGoDebugEnvironmentVariablesFromFeatureflag variables

## Env variables to control connection
## READ_ONLY
## POINT_IN_TIME

## Setup reposotory connections

## TODO: create repository if it doesn't exist? Fail if it doesn't exist?

## FIXME: support other repo types

repo_connect_s3() {
    kopia repository connect s3 \
        --no-check-for-updates \
        --bucket=${storage_bucket} \
        --access-key=${storage_access_key} \
        --secret-access-key=${storage_access_secret} \
        --cache-directory=${cacheDirectory} \
        --override-hostname=${hostname} \
        --override-username=${username} \
        --content-cache-size-limit-mb=${contentCacheSizeMb} \
        --metadata-cache-size-limit-mb=${metadataCacheSizeMb} \
        --endpoint=${storage_endpoint} \
        ${maybe_disable_tls} \
        --prefix=${full_prefix} \
        --log-dir=${logDirectory} \
        --password=${repo_password} \
        --config-file=${configFilePath}
}

repo_connect_filesystem() {
    kopia repository connect filesystem \
        --path=${storage_path_full} \
        --no-check-for-updates \
        --cache-directory=${cacheDirectory} \
        --override-hostname=${hostname} \
        --override-username=${username} \
        --content-cache-size-limit-mb=${contentCacheSizeMb} \
        --metadata-cache-size-limit-mb=${metadataCacheSizeMb} \
        --log-dir=${logDirectory} \
        --password=${repo_password} \
        --config-file=${configFilePath}
}

if [[ "${storage_type}" == "ObjectStore" ]]; then

    object_store_type=$(cat ${storage_secret}/objectStoreType)

    if [[ "${object_store_type}" == "S3" ]]; then

    ## TODO: should we validate that pathType is directory??
    # pathType: Directory

    ## TODO:
    ## skipSSLVerify
    ## protectionPeriod

    storage_bucket=$(cat ${storage_secret}/name)
    storage_prefix=$(cat ${storage_secret}/path)
    storage_region=$(cat ${storage_secret}/region)
    storage_endpoint_full=$(cat ${storage_secret}/endpoint)

    storage_endpoint=${storage_endpoint_full#*://}

    if [[ ${storage_endpoint_full} == *"http://"* ]]; then
        maybe_disable_tls="--disable-tls"
    else
        maybe_disable_tls="--no-disable-tls"
    fi

    ## FIXME: figure this one out
    full_prefix="${storage_prefix}/${rootPath}"

    ## FIXME: support non-aws secret types????
    storage_access_key=$(cat ${storage_creds_secret}/aws_access_key_id)
    storage_access_secret=$(cat ${storage_creds_secret}/aws_secret_access_key)


## FIXME: read only option??
## FIXME: point in time option??
## FIXME: --session-token option
## FIXME: --file-log-level
## TODO: separate common args??
    if ! repo_connect_s3 ; then
        if [[ ${read_only} == "true" ]]; then
            echo "Failed to connect to readonly repository" >&2
            exit 22
        fi
        ## Try to create a repo and re-connect if connect fails
        kopia repository create s3 \
            --bucket=${storage_bucket} \
            --prefix=${full_prefix} \
            --region=${storage_region} \
            --endpoint=${storage_endpoint} \
            ${maybe_disable_tls} \
            --access-key=${storage_access_key} \
            --secret-access-key=${storage_access_secret} \
            --no-check-for-updates \
            --cache-directory=${cacheDirectory} \
            --override-hostname=${hostname} \
            --override-username=${username} \
            --content-cache-size-limit-mb=${contentCacheSizeMb} \
            --metadata-cache-size-limit-mb=${metadataCacheSizeMb} \
            --log-dir=${logDirectory} \
            --password=${repo_password} \
            --config-file=${configFilePath}

        repo_connect_s3
    fi

    else
        echo "Unsupported object store type: ${object_store_type}" >&2
        exit 22
    fi
elif [[ "${storage_type}" == "filesystem" ]]; then

    storage_path=$(cat ${storage_secret}/path)
    storage_mount="/mnt/volumes/data"

    ## FIXME: rootPath for repo prefix!!
    storage_path_full="${storage_mount}/${storage_path}"

    ## FIXME: delete this??
    if ! repo_connect_filesystem ; then
        if [[ ${read_only} == "true" ]]; then
            echo "Failed to connect to readonly repository" >&2
            exit 22
        fi
        ## Create repo and try connect again
        kopia repository create filesystem \
            --path=${storage_path_full} \
            --no-check-for-updates \
            --cache-directory=${cacheDirectory} \
            --override-hostname=${hostname} \
            --override-username=${username} \
            --content-cache-size-limit-mb=${contentCacheSizeMb} \
            --metadata-cache-size-limit-mb=${metadataCacheSizeMb} \
            --log-dir=${logDirectory} \
            --password=${repo_password} \
            --config-file=${configFilePath}
        repo_connect_filesystem
    fi
else
    echo "Unsupported storage type: ${storage_type}" >&2
    exit 22
fi

## Setup client config

client_credentials_source=/etc/client_credentials

## Run background process to refresh users every 2 minutes

refresh_users() {
    ## Cleanup existing users
    kopia server user list --config-file=${configFilePath} | xargs -r -L1 kopia server user delete --config-file=${configFilePath}

    for filename in ${client_credentials_source}/*; do
        ## Read from each file with `:` as a separator, add to kopia users
        kopia_u=$(cut -d":" -f1 ${filename})
        kopia_p=$(cut -d":" -f2 ${filename})
        kopia server user add $kopia_u --no-ask-password --user-password=$kopia_p --config-file=${configFilePath}
    done
}

## Refresh users before starting a server

refresh_users


## Start repository server

server_address=https://0.0.0.0:51515

## TODO: cache dir, cache size same as in repo connect?
## TODO: auto generate cert with --tls-generate-cert??
## TODO: readonly, enable-pprof
## TODO: --metrics-listen-addr

## FIXME: use a different condition (config?) to specify cert source
if [ -f ${tls_cert_file} ]; then
    ## Certificate comes from secrets
    tls_args="--tls-cert-file ${tls_cert_file} --tls-key-file ${tls_key_file}"
else
    ## Certificate is generated on server start
    ## Make certs dir
    mkdir /tmp/kopia-cert
    tls_args="--tls-generate-cert --tls-cert-file /tmp/kopia-cert/tls.cert --tls-key-file /tmp/kopia-cert/tls.key"
    tls_cert_file="/tmp/kopia-cert/tls.cert"
fi

kopia server start \
    --address=${server_address} \
    --cache-directory=${cacheDirectory} \
    --no-check-for-updates \
    --content-cache-size-limit-mb=${contentCacheSizeMb} \
    --metadata-cache-size-limit-mb=${metadataCacheSizeMb} \
    --server-username=${admin_username} \
    --server-password=${admin_password} \
    --server-control-username=${admin_username} \
    --server-control-password=${admin_password} \
    ${tls_args} \
    --log-dir=${logDirectory} \
    --config-file=${configFilePath} &

server_pid=$!

## Advertise server data

## Wait for cert file first:
while [ ! -f ${tls_cert_file} ]; do sleep 1; done

## TODO: we can also get the fingerprint from server start
openssl x509 -in ${tls_cert_file} -noout -fingerprint -sha256 | sed 's/://g' | cut -f 2 -d = > /etc/session/data

# Inform startup probes (after writing server data!)
touch /etc/session/ready

loop_refresh_users() {
    ## TODO: set up monitoring for file changes

    ## FIXME: check if failing background process fails main one
    ## FIXME: redirect logs for user refresh???
    while sleep 120; do
        refresh_users
        ## TODO: refresh command only works if server is running
        kopia server refresh --address=${server_address} --server-cert-fingerprint=$(cat /etc/session/data) --server-control-username=${admin_username} --server-control-password=${admin_password}
    done &
    ## TODO: in case we need a refresh loop PID
    refresh_loop=$!
}

## Run loop refreshing users every 120 sec
loop_refresh_users

wait $server_pid