# Dispatch office requests

I did not change any code for either request. Here is what I did and why.

## a) Licence number on the visit detail page

**What I did:** no change. I asked dispatch for a decision first.

**Why:**

- **The rules do not allow it.** BUSINESS_RULES §1.3 says the licence number is encrypted and shown only to admins, only on `GET /api/users/:id` (`backend/internal/api/user_handler.go:66`). Dispatchers are not admins, and the visit detail page is also shown to the technician on the visit. Showing it there changes a rule about personal data, so the owner of that rule has to agree first.
- **It would not show whether the licence is current.** Only the number is stored. There is no expiry date or status, so dispatch would still have to check it with the body that issued it.

**Questions back:** Who can approve dispatchers seeing licence numbers? If approved, §1.3 is updated and the change is small. And do they really need an expiry date, or a record of when the licence was last checked? That is a bigger change to scope with them.

## b) Removing people who have left from the history

**What I did:** declined. No data was changed.

**Why:**

- **It is a record, not a label.** "Recorded by" says who logged a technician's clock times, which make up the hours in the daily report (§5.3). If those hours are ever questioned, this is the answer.
- **The rules say to keep it.** §4.3 and §1.2 keep the name after the user is deleted. Each clock-in also wrote an audit row (§6.1), and §6.2 says audit rows are never edited or deleted. Removing the name cannot be undone.
- **Most of the "tidy" part already works.** Dana's account is soft-deleted: she cannot log in and does not appear in user lists or the technician filter. Her name shows only on work she did.

**Questions back:** Where does her name get in the way? If it shows up somewhere it should not, that is a bug and I will fix it. If this is a request to erase her personal data, it goes to whoever handles data protection, because it has to be weighed against keeping records of hours worked.
