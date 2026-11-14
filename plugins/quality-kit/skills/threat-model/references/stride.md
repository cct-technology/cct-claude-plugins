# Boundaries and STRIDE

## Trust boundaries

A boundary is where data or a request passes between two sides that trust each other differently. The usual ones:

| Boundary | Look for |
| --- | --- |
| A person's browser or app, and your server | Routes, forms, uploads, anything read from the request |
| One kind of user, and another | Roles, admin pages, actions only some may take |
| One tenant, and another | A team, organisation or customer whose data the others must not see |
| Your service, and an external one | A provider's API you call, with a key or a token |
| An external service calling you | A webhook, a callback, a return address after a redirect |
| Your service, and a language model | Text you send to a model, and what you do with its answer |
| Your code, and a store or a queue shared with other systems | A database others write to, a message from another service |
| Stored content, and the person who later sees it | Text one user writes that another user's browser shows |

**A new external service is a new supplier.** Give it one threat of its own: what happens to the feature and its data when the provider fails, changes or is breached, and what the agreement with it has to say. That row carries a supplier control, A.5.19 to A.5.23.

A boundary earns a row when the feature adds it, or sends something new across it. Five is the most a short threat model holds: join two that are crossed in the same way.

In the **Code** cell, name the files where the boundary is crossed. A later check will use them to tell when the threat model is out of date.

## The six questions

Ask each at each boundary. The answer is a threat only when you can say who does it and what it costs.

| | Question | Typical threats | Typical mitigations |
| --- | --- | --- | --- |
| **S** Spoofing | Can someone pass as another person or system? | A forged webhook, a stolen session, a guessed link | Check a signature, require a sign-in, bind a token to its purpose |
| **T** Tampering | Can someone change data or a request they should not? | A changed price or ID in a request, stored script in a text field | Take values from the server's own record, validate and encode, least privilege on the store |
| **R** Repudiation | Can someone deny what they did, or can you not tell who did it? | A deletion or a refund with no record | Log who did what and when, where the actor cannot change it |
| **I** Information disclosure | Can someone read what is not theirs? | Another tenant's record by its ID, personal data in a log or sent to a provider | Scope every query to the owner, send the least data, keep secrets out of logs and answers |
| **D** Denial of service | Can someone make it unusable or costly for others? | Unbounded input, a costly call anyone can repeat | Limits on size and rate, time-outs, a cap on spend |
| **E** Elevation of privilege | Can someone do what their role does not allow? | A route that checks the sign-in but not the role, an action taken on the server with more rights than the caller has | Check the right at every action, on the server |

## A row that people can act on

- **Threat:** who does what, and what it costs. "Anyone who knows the address posts `payment.succeeded` and gets an order marked paid", not "webhook spoofing".
- **Mitigation:** one concrete thing. "Check the event's signature with Paywell's signing secret before reading it", not "validate input".
- **Status `in place`** needs proof: the mitigation names the file that shows it. When you cannot point at the code, the status is `to do`. A spec that promises something the code does not do yet is `to do`.
- **Leave out** what the platform already settles for every feature the same way, unless this feature weakens it.

## The checklist

One line for each row that is `to do`: the row's ID, then the work in one sentence someone can pick up without reading the table. Order the lines by cost: what loses money or leaks data first.

## Accepting a risk

Some threats are not worth their mitigation. That is a person's call. Say in the report which rows are candidates and why, and leave **Risks accepted** as the template has it.
