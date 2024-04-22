# trade-tariff-lambdas-status-checks

Scheduled go lambda function to check the P90 status of the various services and store the results in an status.json file in s3 for review on a static status page

```mermaid
sequenceDiagram
    participant C as Cloudwatch Event
    participant L as Lambda Function
    participant S3 as S3 Bucket
    participant CF as Cloudfront Distribution
    participant U as User

    C->>+L: Triggers every 5 minutes
    L->>+L: Iterates through P90 synthetics
    L->>+S3: Drops status.json update file
    S3->>-CF: Serves index.html via Cloudfront
    CF->>U: User accesses status page
    U->>U: Sees status of each application
```
