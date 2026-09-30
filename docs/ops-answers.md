# Operations answers

## a) Incident: `column "priority" does not exist` on `/api/visits`

Some facts about this system shape the answer:

- A backend instance migrates the database at boot and exits if that fails (`backend/main.go:48`). So new code should never serve without the column. If it does, the first question is which database and which image the failing instances use, not why a migration failed.
- `GET /api/health` always returns 200 and never touches the database (`backend/internal/api/handlers.go:29`). The load balancer cannot see this failure, so the deploy can finish "healthy" while visits are broken.
- The old backend never reads `priority`, so it works whether the column exists or not. The new frontend only shows priority, and with the old backend that cell is just empty.

**First thirty minutes, in order**

1. **Confirm the scope (0 to 5 min).** Check the error in the logs on more than one instance (`docker logs fieldops-backend` via Session Manager). Is it every instance or only the new ones? Only `/api/visits`, or other paths too?
2. **Stop the rollout from spreading.** If the instance refresh is still running, cancel it so no more old instances are replaced.
3. **Tell the dispatch office (by minute 5 to 10).** See below.
4. **Roll the backend back (10 to 20 min).** Point the auto-scaling group at the previous launch template version (the previous image) and run an instance refresh. This is the fastest safe fix because the old code does not need the column. Leave the frontend as it is.
5. **Check it is fixed (20 to 30 min).** `/api/visits` returns 200 on every instance, the error is gone from the logs, and someone in dispatch confirms they can see the visits list.
6. **Then look for the cause, read only.** Which `DATABASE_URL` the instances got from SSM, which image tag they run, what their boot logs say about the migration, and whether `visits` has the column on that database.

**What would change my mind:** if the previous image is not available, or rolling back fails, I would roll forward instead. That means a reviewed fix through the normal deploy, once I know which database is wrong. Rolling back loses no data, because the old code never reads or writes `priority`.

**What I tell the dispatch office, and when**

Within the first five to ten minutes, in plain words. For example: "Since about 14:10 the visits list is not loading. It started with an update we released, and we are undoing that update now. Technicians may not see their visits until then, so please give them their jobs by phone. Next update at 14:30." Then an update when it is fixed, and a short note later on what happened.

**What I deliberately do not do**

- Add the column by hand in `psql` on production. I do not yet know which database is wrong, and a hand change is not reviewed or recorded anywhere.
- Restore from a backup. No data is lost, and a restore would throw away the visits booked since the backup.
- Run `terraform apply`. A deploy is not a Terraform change (`docs/OPERATIONS.md`).
- Push a quick fix from an unreviewed branch, or roll back the frontend, which is not broken.
- Hunt for the root cause while the service is still down. Fix first, investigate after.

## b) Restore

On the seeded local database:

```bash
make db-dump                                                # backups/fieldops-20260930-113335.sql
make db-restore FILE=backups/fieldops-20260930-113335.sql
```

```
    table     | rows
--------------+------
 audit_logs   |    1
 clock_events |   93
 sites        |    6
 users        |    6
 visits       |   92
(5 rows)
```

The live database gave the same five counts with the same query.

**Production (RDS), following `docs/OPERATIONS.md`:**

1. Choose the restore point: a daily snapshot, or a point in time within the 7-day backup window.
2. Restore it to a **new** RDS instance in the same subnet group and security group. Never restore over the live instance.
3. Check the new instance (below) before anything uses it.
4. Put the new address in `/fieldops/<env>/DATABASE_URL` in SSM and start an instance refresh. Instances read SSM only at boot.
5. Keep the old instance until the new one is confirmed. Then bring the new instance into Terraform, because Terraform still points at the old one.

**How I would know the restore is good before switching:**

- Row counts per table are close to what production had, and the newest `visits.created_at` and `clock_events.occurred_at` match the chosen restore point.
- The schema matches the build that will run on it, for example `visits` has `priority`.
- One backend container, run by hand against the new database (not in the auto-scaling group), can list visits, open the daily report for a known day, and show an admin a user's licence number. The last check proves the current `ENCRYPTION_KEY` still decrypts the restored data.

## c) Infrastructure

**Where the three secrets live.** `DATABASE_URL`, `JWT_SECRET` and `ENCRYPTION_KEY` are SSM Parameter Store SecureStrings under `/fieldops/<env>/`. They are created by hand, not by Terraform (`infra/README.md`, apply step 3). At boot each instance reads them (`infra/modules/compute/user_data.sh.tpl`), writes them to `/etc/fieldops/backend.env` (mode 0600), and passes that file to the container. The instance role can read only `/fieldops/<env>/*`.

**Rotating `JWT_SECRET` without downtime.** With the current code it cannot be done. The backend holds one secret, used both to sign and to check tokens (`backend/internal/services/auth.go:24` and `:110`). I tested this locally: after restarting with a new secret, an existing token got 401 `invalid or expired token`. So a rotation logs everyone out. During a rolling refresh it is worse: old and new instances hold different secrets and the load balancer has no sticky sessions, so users get random 401s depending on which instance answers.

To make it possible, change the code first so the backend checks tokens against a list of secrets and signs with the first one. Then:

1. Add the new secret as a second secret for checking only, and deploy.
2. Make the new secret the signing one, and deploy.
3. Wait 8 hours, the token lifetime (§1.2), so every old token has expired.
4. Remove the old secret, and deploy.

Until that change exists, the honest option is a planned rotation at a quiet time, with everyone warned that they will need to log in again.

**What `terraform destroy` would take, and what would survive.** Terraform deletes everything it can and reports errors for the rest.

- **Deleted:** the auto-scaling group and its instances, the launch template, the load balancer, the instance role, the CloudFront distribution and the NAT gateway. The site goes down.
- **Survives, because the delete fails:** the RDS instance (`deletion_protection`), and with it the private subnets, security groups and VPC it still uses. The S3 bucket also fails to delete if it has files in it, because it has no `force_destroy`.
- **Survives, because Terraform does not manage it:** the ECR repository and images, and the three SSM secrets.

So the data is safe, but the service is down and the rest is a half-deleted stack to clean up by hand.

**Why `deletion_protection` is on the database.** The database is the one thing Terraform cannot rebuild. Everything else can be recreated from code in minutes, but the data cannot. Protection stops a delete from any source: `terraform destroy`, the console, or a plan that quietly replaces the instance, for example after renaming `identifier` or `username`. `skip_final_snapshot = false` is a second safety net. Without protection, deleting the instance would also remove its 7 days of automated backups by default, leaving only the final snapshot.
