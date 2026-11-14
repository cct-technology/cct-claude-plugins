# ISO/IEC 27001:2022 Annex A controls

Tag each threat with the one control its mitigation serves most directly. The tag lets the threat model count as evidence for that control. Use the number and, the first time, the name.

| The mitigation is about | Control |
| --- | --- |
| Who may use what; the rules of access | A.5.15 Access control |
| Accounts and identities, their whole life | A.5.16 Identity management |
| Passwords, keys, tokens and other secrets used to sign in | A.5.17 Authentication information |
| Granting, changing and removing rights | A.5.18 Access rights |
| Admin and other high rights | A.8.2 Privileged access rights |
| Each user or tenant sees only their own data | A.8.3 Information access restriction |
| Signing in: sessions, expiry, a second factor | A.8.5 Secure authentication |
| One person must not both do and approve | A.5.3 Segregation of duties |
| Sending data to another party safely | A.5.14 Information transfer |
| Personal data: least data, purpose, the data subject's rights | A.5.34 Privacy and protection of personal identifiable information (PII) |
| Hiding part of the data from those who do not need it | A.8.11 Data masking |
| Keeping data from leaving through logs, answers or exports | A.8.12 Data leakage prevention |
| Removing data when it is no longer needed | A.8.10 Information deletion |
| Encryption, signatures and their keys | A.8.24 Use of cryptography |
| Recording who did what, and protecting the record | A.8.15 Logging |
| Watching for misuse and reacting | A.8.16 Monitoring activities |
| Limits on size, rate and spend; enough capacity | A.8.6 Capacity management |
| Staying in service when a part fails | A.8.14 Redundancy of information processing facilities |
| Protecting the network and what is reachable from where | A.8.20 Networks security |
| Security requirements of the application: validation, limits, session rules | A.8.26 Application security requirements |
| Writing the code safely: encoding output, parameters in queries | A.8.28 Secure coding |
| Designing so that one failure does not open everything | A.8.27 Secure system architecture and engineering principles |
| Testing that the mitigation works | A.8.29 Security testing in development and acceptance |
| Settings and defaults that are safe | A.8.9 Configuration management |
| Known flaws in what you depend on | A.8.8 Management of technical vulnerabilities |
| Choosing and checking a supplier | A.5.19 Information security in supplier relationships |
| What the agreement with a supplier must say | A.5.20 Addressing information security within supplier agreements |
| Packages, models and other parts from others | A.5.21 Managing information security in the information and communication technology (ICT) supply chain |
| Watching a supplier's service, and its changes | A.5.22 Monitoring, review and change management of supplier services |
| A cloud or hosted service, a hosted model among them | A.5.23 Information security for use of cloud services |

When two controls fit, take the one the mitigation acts on: a signature check on a webhook is A.8.24, the agreement that the provider signs its events is A.5.20.
