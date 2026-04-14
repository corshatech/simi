```
helm install dln . -n caldln --create-namespace --set action=create-network
helm upgrade dln . -n caldln --create-namespace --set action=scale
```
