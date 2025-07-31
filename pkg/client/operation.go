package client

import (
	"path"

	corev1 "k8s.io/api/core/v1"
)

const (
	// FIXME: put en env variable for stream file for blueprints convenience
	StreamFileDir          = "/tmp/stream_file/"
	StreamFileName         = "data"
	streamDefaultInitImage = "busybox:latest"
)

const (
	OpFsBackup      = "fs_backup"
	OpFsRestore     = "fs_restore"
	OpFsSidecar     = "fs_sidecar"
	OpStreamBackup  = "stream_backup"
	OpStreamRestore = "stream_restore"
)

type Operation interface {
	MakeVolumes() ([]corev1.Volume, []corev1.VolumeMount, []corev1.VolumeDevice)
	MakeArgs() []string
	MakeContainers() []corev1.Container
	MakeInitContainers() []corev1.Container
}

var _ Operation = FileSystemBackupOperation{}
var _ Operation = FileSystemRestoreOperation{}

// FileSystemBackupOperation backs up file system directory at Path from PVC
type FileSystemBackupOperation struct {
	Path string
	// TODO: do we want to have extra metadata other than tag?
	Tag           string
	PVC           string
	ReadOnlyMount bool
}

// FileSystemRestoreOperation restores file system directory to Path in PVC
type FileSystemRestoreOperation struct {
	Path     string
	BackupID string
	PVC      string
}

// FileSystemSidecarOperation runs in sidecar containers and reads
// backup or restore commands from a file descriptor with FileName
// This operation is used when datamover container is running
// as a sidecar to existing pod with data.
type FileSystemSidecarOperation struct {
	FileName    string
	VolumeMount corev1.VolumeMount
}

var _ Operation = StreamBackupOperation{}
var _ Operation = StreamRestoreOperation{}

// StreamBackupOperation backs up data from StreamFile file
// StreamGenerator container should generate data and send it to a file in StreamFileDir
type StreamBackupOperation struct {
	// TODO: do we want to have extra metadata other than tag?
	Tag string
	// TODO: how do we control vitrual file names??
	StreamGenerator corev1.Container
	// Identifier of a stream data in a backup (filename)
	BackupObjectName string
	// Optional: image to use for init container
	InitImage string
}

// StreamRestoreOperation restores data and writes in into a file in StreamFileDir
// StreamIngestor container should read from this file and perform a restore
type StreamRestoreOperation struct {
	BackupID string
	// TODO: PERFORMANCE TUNING
	StreamIngestor corev1.Container
	// Identifier of a stream data in a backup (filename)
	BackupObjectName string
	// Optional: image to use for init container
	InitImage string
}

// Example of another operation working with FLR APIs
// type StartFLRServerOperation struct {
// 	// TODO: what do we even need here?
// }

// StreamBackupOperation generates EmptyDir volume for communication between
// the main container and data generator container
// It also returns all configured volumes (for use in the generator) container
func (streamBackup StreamBackupOperation) MakeVolumes() ([]corev1.Volume, []corev1.VolumeMount, []corev1.VolumeDevice) {
	streamFileVolume, streamFileVolumeMount := makeStreamFileVolume()
	return []corev1.Volume{streamFileVolume}, []corev1.VolumeMount{streamFileVolumeMount}, nil
}

// StreamRestoreOperation generates EmptyDir volume for communication between
// the main container and data ingestor container
// It also returns all configured volumes (for use in the ingestor) container
func (streamRestore StreamRestoreOperation) MakeVolumes() ([]corev1.Volume, []corev1.VolumeMount, []corev1.VolumeDevice) {
	streamFileVolume, streamFileVolumeMount := makeStreamFileVolume()
	return []corev1.Volume{streamFileVolume}, []corev1.VolumeMount{streamFileVolumeMount}, nil
}

func makeStreamFileVolume() (corev1.Volume, corev1.VolumeMount) {
	return corev1.Volume{
			Name: "stream-file",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium: corev1.StorageMediumMemory,
					// TODO: do we need to set the size limit?
				},
			},
		}, corev1.VolumeMount{
			Name:      "stream-file",
			MountPath: StreamFileDir,
		}
}

// File system sidecar operation relies on volume to be in the pod and returns a mount to it
func (monitorFile FileSystemSidecarOperation) MakeVolumes() ([]corev1.Volume, []corev1.VolumeMount, []corev1.VolumeDevice) {
	return []corev1.Volume{}, []corev1.VolumeMount{monitorFile.VolumeMount}, nil
}

// Backup and restore operations generate "data" volume from PVC and mount it to /mnt/data/data
func (restore FileSystemRestoreOperation) MakeVolumes() ([]corev1.Volume, []corev1.VolumeMount, []corev1.VolumeDevice) {
	return makeDataPvcVolumes(restore.PVC, false)
}

func (backup FileSystemBackupOperation) MakeVolumes() ([]corev1.Volume, []corev1.VolumeMount, []corev1.VolumeDevice) {
	return makeDataPvcVolumes(backup.PVC, backup.ReadOnlyMount)
}

func makeDataPvcVolumes(pvc string, readOnly bool) ([]corev1.Volume, []corev1.VolumeMount, []corev1.VolumeDevice) {
	return makePvcVolumes("/mnt/data/", readOnly, map[string]string{"data": pvc})
}

func makePvcVolumes(prefix string, readOnly bool, pvcMap map[string]string) ([]corev1.Volume, []corev1.VolumeMount, []corev1.VolumeDevice) {
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	for name, pvc := range pvcMap {
		volumes = append(volumes, corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc,
					ReadOnly:  readOnly,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      name,
			MountPath: prefix + name,
		})
	}

	return volumes, volumeMounts, nil
}

func (streamBackup StreamBackupOperation) MakeArgs() []string {
	return []string{
		OpStreamBackup,
		streamFileName(),
		streamBackup.BackupObjectName,
		streamBackup.Tag,
	}
}

func (streamRestore StreamRestoreOperation) MakeArgs() []string {
	return []string{
		OpStreamRestore,
		streamFileName(),
		streamRestore.BackupObjectName,
		streamRestore.BackupID,
	}
}

func streamFileName() string {
	return path.Join(StreamFileDir, StreamFileName)
}

func (backup FileSystemBackupOperation) MakeArgs() []string {
	return []string{
		OpFsBackup,
		backup.Path,
		backup.Tag,
	}
}

func (restore FileSystemRestoreOperation) MakeArgs() []string {
	return []string{
		OpFsRestore,
		restore.Path,
		restore.BackupID,
	}
}

func (sidecar FileSystemSidecarOperation) MakeArgs() []string {
	return []string{
		OpFsSidecar,
		sidecar.FileName,
	}
}

// TODO: potentially inject more performance tunning fields in the container
func (streamBackup StreamBackupOperation) MakeContainers() []corev1.Container {
	// FIXME: container name
	streamGeneratorContainer := streamBackup.StreamGenerator
	return []corev1.Container{
		appendStreamFileMount(streamGeneratorContainer),
	}
}

func (streamRestore StreamRestoreOperation) MakeContainers() []corev1.Container {
	// FIXME: container name
	streamIngestorContainer := streamRestore.StreamIngestor
	return []corev1.Container{
		appendStreamFileMount(streamIngestorContainer),
	}
}

func appendStreamFileMount(container corev1.Container) corev1.Container {
	streamFileMount := streamVolumeMount()
	container.VolumeMounts = append(container.VolumeMounts, streamFileMount)
	return container
}
func (fsBackup FileSystemBackupOperation) MakeContainers() []corev1.Container   { return nil }
func (fsRestore FileSystemRestoreOperation) MakeContainers() []corev1.Container { return nil }
func (sidecar FileSystemSidecarOperation) MakeContainers() []corev1.Container   { return nil }

func (streamBackup StreamBackupOperation) MakeInitContainers() []corev1.Container {
	initImage := streamBackup.InitImage
	return []corev1.Container{streamInitContainer(initImage)}
}

func (streamRestore StreamRestoreOperation) MakeInitContainers() []corev1.Container {
	initImage := streamRestore.InitImage
	return []corev1.Container{streamInitContainer(initImage)}
}

func (fsBackup FileSystemBackupOperation) MakeInitContainers() []corev1.Container   { return nil }
func (fsRestore FileSystemRestoreOperation) MakeInitContainers() []corev1.Container { return nil }
func (sidecar FileSystemSidecarOperation) MakeInitContainers() []corev1.Container   { return nil }

func streamInitContainer(initImage string) corev1.Container {
	return corev1.Container{
		Name:         "initstreamfile",
		Command:      []string{"mkfifo", streamFileName()},
		Image:        streamInitImage(initImage),
		VolumeMounts: []corev1.VolumeMount{streamVolumeMount()},
	}
}

func streamInitImage(image string) string {
	if image == "" {
		return streamDefaultInitImage
	} else {
		return image
	}
}

func streamVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "stream-file",
		MountPath: StreamFileDir,
	}
}
