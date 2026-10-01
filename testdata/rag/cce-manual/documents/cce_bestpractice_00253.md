# 通过StorageClass动态创建SFS Turbo子目录

来源：https://support.huaweicloud.com/usermanual-cce/cce_bestpractice_00253.html
目录：用户指南 > 存储 > 极速文件存储（SFS Turbo） > 通过StorageClass动态创建SFS Turbo子目录
更新时间：2026-09-20 GMT+08:00

#### 背景信息

在创建SFS Turbo时有最低容量限制，并非按实际的使用量进行计费。而默认情况下，工作负载在挂载SFS Turbo时将根目录映射至容器，负载实际使用的容量远低于这一限值，易造成资源浪费。

everest插件支持一种在SFS Turbo下动态创建子目录的方法，能够在SFS Turbo下动态创建子目录并挂载到容器，这种方法能够共享使用SFS Turbo，从而更加经济合理地利用SFS Turbo存储容量。

#### 约束与限制

- 仅支持1.15+集群。
- 集群必须使用everest插件，插件版本要求1.1.13+。
- 不支持安全容器。
- 使用everest 1.2.69之前或2.1.11之前的版本时，使用子目录功能时不能同时并发创建超过10个PVC。推荐使用everest 1.2.69及以上或2.1.11及以上的版本。
- subpath类型的卷实际为SFS Turbo的子目录，对该类型的PVC进行扩容仅会调整PVC声明的资源范围，并不会调整SFS Turbo资源的总容量。若SFS Turbo资源总容量不足，subpath类型卷的实际可使用的容量大小也会受限，您需要前往SFS Turbo界面进行扩容。

  同理，删除subpath类型的卷也不会实际删除后端的SFS Turbo资源。

#### 创建subpath类型SFS Turbo存储卷

1. 创建SFS Turbo资源，选择网络时，请选择与集群相同的VPC与子网。
2. 新建一个StorageClass的YAML文件，例如sfsturbo-subpath-sc.yaml。

   配置示例：

   ```
   apiVersion: storage.k8s.io/v1
   allowVolumeExpansion: true
   kind: StorageClass
   metadata:
     name: sfsturbo-subpath-sc #storageclass的名称
   mountOptions:  #mount挂载参数
   - lock
   parameters:
     csi.storage.k8s.io/csi-driver-name: sfsturbo.csi.everest.io
     csi.storage.k8s.io/fstype: nfs
     everest.io/archive-on-delete: "false"
     everest.io/share-expand-type: bandwidth
     everest.io/share-export-location: 192.168.1.1:/sfsturbo/  #挂载目录配置
     everest.io/share-source: sfs-turbo
     everest.io/share-volume-type: STANDARD
     everest.io/volume-as: subpath
     everest.io/volume-id: 0d773f2e-1234-1234-1234-de6a35074696  #SFS Turbo资源的卷ID
   provisioner: everest-csi-provisioner
   reclaimPolicy: Delete
   volumeBindingMode: Immediate
   ```

   其中：

   - name：storageclass的名称。
   - mountOptions：选填字段；mount挂载参数。
     - everest 1.2.8以下，1.1.13以上版本仅开放对nolock参数配置，mount操作默认使用nolock参数，无需配置。nolock=false时，使用lock参数。
     - everest 1.2.8及以上版本支持更多参数，默认使用如下所示配置，具体请参见[设置挂载参数](https://support.huaweicloud.com/usermanual-cce/cce_10_0626.html)。**此处****不能****配置为nolock=true，会导致挂载失败。**

       ```
       mountOptions:
       - vers=3
       - timeo=600
       - nolock
       - hard
       ```
   - everest.io/volume-as：该参数需设置为“subpath”来使用subpath模式。
   - everest.io/share-expand-type：选填字段。若SFS Turbo资源存储类型为增强版（标准型增强版、性能型增强版），设置为bandwidth。
   - everest.io/share-export-location：挂载目录配置。由SFS Turbo共享路径和子目录组成，共享路径可至SFS Turbo服务页面查询，子路径由用户自行创建，后续指定该StorageClass创建的PVC均位于该子目录下。
   - everest.io/share-volume-type：选填字段。填写SFS Turbo的类型。标准型为STANDARD，性能型为PERFORMANCE。对于增强型需配合“everest.io/share-expand-type”字段使用，everest.io/share-expand-type设置为“bandwidth”。
   - everest.io/zone：选填字段。指定SFS Turbo资源所在的可用区。
   - everest.io/volume-id：SFS Turbo资源的卷ID，可至SFS Turbo界面查询。
   - everest.io/archive-on-delete：若该参数设置为“true”，在回收策略为“Delete”时，删除PVC会删除子目录，并将PV的原文档进行归档，归档目录的命名规则“archived-$pv名称.时间戳”。（特殊情况：在集群删除时，不会删除子目录，且不进行归档）。该参数设置为“false”时，会将PV对应的SFS Turbo子目录删除，不进行归档。默认设置为“true”。

3. 执行**kubectl create -f sfsturbo-subpath-sc.yaml**。
4. 新建一个PVC的YAML文件，sfs-turbo-test.yaml。

   配置示例：

   ```
   apiVersion: v1
   kind: PersistentVolumeClaim
   metadata:
     name: sfs-turbo-test  #PVC的名称
     namespace: default
   spec:
     accessModes:
     - ReadWriteMany
     resources:
       requests:
         storage: 50Gi
     storageClassName: sfsturbo-subpath-sc  #SC的名称
     volumeMode: Filesystem
   ```

   其中：

   - name: PVC的名称。
   - storageClassName：SC的名称。
   - storage：subpath模式下，调整该参数的大小不会对SFS Turbo容量进行调整。实际上，subpath类型的卷是SFS Turbo中的一个文件路径，因此在PVC中对subpath类型的卷扩容时，不会同时扩容SFS Turbo资源。

     ![](https://support.huaweicloud.com/intl/zh-cn/usermanual-cce/public_sys-resources/note_3.0-zh-cn.png)

     subpath子目录的容量受限于SFS Turbo资源的总容量，若SFS Turbo资源总容量不足，请您及时到SFS Turbo界面调整。

5. 执行**kubectl create -f sfs-turbo-test.yaml**。

#### 创建Deployment挂载已有数据卷

1. 新建一个Deployment的YAML文件，例如deployment-test.yaml。

   配置示例：

   ```
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: test-turbo-subpath-example   #创建的工作负载名称
     namespace: default
     generation: 1
     labels:
       appgroup: ''
   spec:
     replicas: 1
     selector:
       matchLabels:
         app: test-turbo-subpath-example
     template:
       metadata:
         labels:
           app: test-turbo-subpath-example
       spec:
         containers:
         - image: nginx:latest  #工作负载的镜像
           name: container-0
           volumeMounts:
           - mountPath: /tmp   #容器内挂载路径
             name: pvc-sfs-turbo-example
         restartPolicy: Always
         imagePullSecrets:
         - name: default-secret
         volumes:
         - name: pvc-sfs-turbo-example
           persistentVolumeClaim:
             claimName: sfs-turbo-test   #已有的PVC名称
   ```

   其中：

   - name：创建的工作负载名称。
   - image：工作负载的镜像。
   - mountPath：容器内挂载路径，示例中挂载到“/tmp”路径。
   - claimName：已有的PVC名称。

2. 创建Deployment负载。

   **kubectl create -f deployment-test.yaml**

#### StatefulSet动态创建subpath模式的数据卷

1. 新建一个StatefulSet的YAML文件，例如statefulset-test.yaml。

   配置示例：

   ```
   apiVersion: apps/v1
   kind: StatefulSet
   metadata:
     name: test-turbo-subpath #创建的工作负载名称
     namespace: default
     generation: 1
     labels:
       appgroup: ''
   spec:
     replicas: 2
     selector:
       matchLabels:
         app: test-turbo-subpath
     template:
       metadata:
         labels:
           app: test-turbo-subpath
         annotations:
           metrics.alpha.kubernetes.io/custom-endpoints: '[{"api":"","path":"","port":"","names":""}]'
           pod.alpha.kubernetes.io/initialized: 'true'
       spec:
         containers:
           - name: container-0
             image: 'nginx:latest'  #工作负载的镜像
             resources: {}
             volumeMounts:
               - name: sfs-turbo-160024548582479676
                 mountPath: /tmp  #容器内挂载路径
             terminationMessagePath: /dev/termination-log
             terminationMessagePolicy: File
             imagePullPolicy: IfNotPresent
         restartPolicy: Always
         terminationGracePeriodSeconds: 30
         dnsPolicy: ClusterFirst
         securityContext: {}
         imagePullSecrets:
           - name: default-secret
         affinity: {}
         schedulerName: default-scheduler
     volumeClaimTemplates:
       - metadata:
           name: sfs-turbo-160024548582479676
           namespace: default
           annotations: {}
         spec:
           accessModes:
             - ReadWriteMany
           resources:
             requests:
               storage: 10Gi
           storageClassName: sfsturbo-subpath-sc #填写自建的SC名称
     serviceName: wwww
     podManagementPolicy: OrderedReady
     updateStrategy:
       type: RollingUpdate
     revisionHistoryLimit: 10
   ```

   其中：

   - name：创建的工作负载名称。
   - image：工作负载的镜像。
   - mountPath：容器内挂载路径，示例中挂载到“/tmp”路径。
   - “spec.template.spec.containers.volumeMounts.name ”和 “spec.volumeClaimTemplates.metadata.name”有映射关系，必须保持一致。
   - storageClassName: 填写自建的SC名称。

2. 创建StatefulSet负载。

   **kubectl create -f statefulset-test.yaml**
