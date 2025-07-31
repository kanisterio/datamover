#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

echo "ARGS: $@"

## Check protocols (early exit)

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

## Data volume mount

## TODO: data/data is a bit redundant, change that if we chose to support only one volume
data_mount=/mnt/data/data

connect_to_repo() {
    ## Read client config

    ## FIXME: do we even need that for kopia??
    config_dir=/etc/config

    # parallelism=${PARAM_PARALLELISM}

    ## FIXME env vars:
    # "CONTENT_CACHE_MB"
    # "METADATA_CACHE_MB"
    # "CONFIG_FILE"
    # "LOG_DIR"
    # "CACHE_DIR"
    # "LOG_LEVEL"

    ## Read client secrets

    credentials_dir=/etc/client-secret

    ## FIXME: do we need to pick specific client ID here??
    ## Assuming there's only one file in there
    credentials_filename=$(ls ${credentials_dir})
    credentials_file=${credentials_dir}/${credentials_filename}

    ## Parse user@host:password credential string

    host_user=$(cut $credentials_file -d":" -f1)
    password=$(cut $credentials_file -d":" -f2)

    username=$(echo $host_user | cut -d"@" -f1)
    hostname=$(echo $host_user | cut -d"@" -f2)

    ## TODO: do we need to refresh that for client??
    ## TODO: maybe we need to wait for the file to be available?

    ## Read additional secrets
    ## FIXME: do we even need that for kopia??

    ## TLS fingerprint is in server data

    tls_fingerprint=$(echo -n $SESSION_DATA | base64 -d)

    ## TODO: do we want to support connecting to non-server kopia??

    ## TODO: do we want to pass config parameters (e.g. log, cache dir etc)

    kopia repository connect server \
        --url https://${SESSION_URL:?}:${port:?} \
        --server-cert-fingerprint ${tls_fingerprint:?} \
        --no-check-for-updates \
        --override-hostname=${hostname:?} \
        --override-username=${username:?} \
        --password=${password:?}
        # --parallelism...
}

# backup
run_backup() {
    local path_prefix=${1:?"Path prefix required"}
    local tags=${2:-}

    local tags_arg=""
    if [[ $tags ]]; then
        tags_arg="--tags ${tags}"
    fi

    ## Connect to repo
    connect_to_repo

    ## FIXME: do we start prefix with /???
    ## TODO: do we want to pass config parameters (e.g. log, cache dir etc)
    ## TODO: parallelism, progress, etc
    ## FIXME: make json parameter optional (env variable)??
    kopia snapshot create --json ${data_mount}/${path_prefix} $tags_arg

    ## TODO: parse output??
}
# restore
run_restore() {
    local path_prefix=$1
    local backup_id=$2

    ## Connect to repo
    connect_to_repo


    ## FIXME: do we start prefix with /???
    ## TODO: sparseRestore
    kopia snapshot restore ${backup_id} ${data_mount}/${path_prefix}
    ## TODO: parse output??
}

## Check command arguments

command=$1

case $command in
    "fs_backup")
        run_backup ${@:2}
        ;;
    "fs_restore")
        run_restore ${@:2}
        ;;
    *)
        echo "Not supported command ${command}"
        exit 1
        ;;
esac

