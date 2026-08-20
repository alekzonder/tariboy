# Tariboy internal alpha feedback

Use one copy per observed onboarding, then summarize weekly. Do not include
prompts, credentials, source code, or customer data.

## Session record

- Date and timezone:
- Participant GitHub username:
- Observer GitHub username:
- App version and commit:
- macOS version and hardware:
- Remote platform:
- Harness:
- New user or returning user:

## Ten-minute funnel

Record elapsed time and whether the observer took control.

| Step | Completed | Elapsed | Blocker or hesitation |
| --- | --- | --- | --- |
| Install and open |  |  |  |
| Add SSH alias |  |  |  |
| Pass preflight/provision |  |  |  |
| Select or build image |  |  |  |
| Create agent |  |  |  |
| Open Console |  |  |  |
| Enable Autopilot |  |  |  |
| Find Activity and cost |  |  |  |
| Find Pause and Kill |  |  |  |

Outcome:

- [ ] completed without author control in ten minutes;
- [ ] completed with verbal guidance only;
- [ ] blocked;
- [ ] stopped for a safety concern.

## Comprehension

Ask the participant to point to each item without coaching:

- [ ] selected host;
- [ ] selected image;
- [ ] Interactive state;
- [ ] Autopilot state;
- [ ] trigger for the latest iteration;
- [ ] current iteration and deadline;
- [ ] outcome of the previous iteration;
- [ ] usage and cost;
- [ ] Pause;
- [ ] Kill.

Ask in their own words:

1. What is the difference between an image and an agent?
2. What happens when Autopilot is disabled?
3. What does removing a host do remotely?
4. When would you use `bare:latest`?
5. What would make you trust this with a real task?

## Weekly interview

- Most valuable moment:
- Most confusing term:
- Step that felt risky:
- Missing control or evidence:
- Current tools this could replace:
- Task they would try next:
- Reason they would not return next week:
- One product change with the highest leverage:

## Manual weekly metrics

- partners invited;
- DMG downloads confirmed;
- first launches;
- SSH hosts connected;
- image builds completed;
- agents created;
- Console sessions started;
- Autopilot iterations completed;
- users who found Activity/cost/Pause/Kill unaided;
- support incidents;
- weekly retained partners.

## Follow-up

Record one issue per blocking failure in the repository. Add a sanitized support
bundle only through the approved internal channel. Assign a repository
collaborator, severity, release impact, and next observation date in the issue.
