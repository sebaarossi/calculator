# Prompt Log and AI-Augmented Engineering

## 📌 Methodology

This project was built with [Claude Code](https://claude.com/claude-code) in an
"AI pair programming" setup. I acted as the software architect: I set the
direction, asked for reviews from specific points of view (tech lead, FinTech
principal engineer, QA), chose between the options I was given and decided what
to accept. The assistant wrote the code and the tests, ran them, and verified
the application in a browser.

How to read this log:

- **Prompts are verbatim.** I work in Spanish, so each one is followed by an
  English translation.
- **Decision taken** is what I chose, and whether the idea was mine or a
  proposal from the assistant that I approved.
- **Result and iteration** is what came out of it, including the defects that
  testing found along the way.
- Purely operational messages (which git commands to run, restarting a local
  server) are left out. Everything that shaped the product is here.

---

### 1. Initial build

**Prompt:**

> tengo que hacer una calculadora, tanto front end como back, como un assesment
> para una entrevista de trabajo. La consigna es la siguiente
>
> _(followed by the full text of the assignment)_

_In English: I have to build a calculator, frontend and backend, as an
assessment for a job interview. The brief is the following._

**Result and iteration:** A first working version: a Go REST API with the
domain logic separated from the HTTP layer, a React + TypeScript frontend,
unit tests on both sides, a Dockerfile, a Makefile and CI.

### 2. Engineering standards and API documentation

**Prompt:**

> Tene en consideracion como si fueses un Techincal Leader, hay que velar por las
> buenas practicas, la legibilidad, el entendimiento. No complicar de mas. Usemos
> swagger para poder ver los endpoints backend

_In English: act as a technical leader would; look after good practices,
readability and understandability; do not over-complicate; use Swagger to
browse the backend endpoints._

**Decision taken:** "Do not over-complicate" became the rule for the rest of
the project. For Swagger, the assistant proposed a hand-written OpenAPI file
embedded in the binary instead of a code generator, to avoid annotations, a
codegen step and extra dependencies for a handful of endpoints. I accepted.

**Result and iteration:** Swagger UI at `/docs`, plus a test that fails if an
operation or route is missing from the specification, so the documentation
cannot silently drift from the code.

### 3. Visual identity

**Prompt:**

> me gustaria darle un poco de personalidad. Con otros colores, quizas no hacerla
> tan calculadora tipica, sino algo mas divertido e innovador

_In English: I would like to give it some personality; different colours,
less of a typical calculator, something more fun and innovative._

**Decision taken:** The assistant proposed a bold, flat visual style and a
paper tape that prints each calculation and lets a result be reused. I kept
both.

**Result and iteration:** Testing the tape exposed a real defect: `2 ^ 40` was
displayed as `1099511627780` instead of `1099511627776`, because the display
rounded to 12 significant digits. It was fixed and covered by a regression
test, and it was the first sign that numeric precision needed proper attention
(see phase 6).

### 4. Repository and API design review

**Prompts:**

> esta bien asi? o es mejor que sean 2 repos?

> no es mejor hacer una branch inicial?

> esta bien ese endpoint nomas? no es mejor uno por operacion? o queda medio engorroso?

_In English: is one repository fine, or are two better? Would an initial
branch be better? Is a single endpoint fine, or is one per operation better?_

**Decision taken:** I challenged three structural choices and, after weighing
the trade-offs, kept all three: one repository (the brief asks for one, and the
frontend, backend and API contract change together); the first commit directly
on `main`, as a sequence of small commits; and a single resource for
calculations instead of one endpoint per operation, since every operation
shares the same input, output and errors.

**Result and iteration:** No code changed. The reasoning is recorded under
"Design decisions" in the README, including the cost of the single endpoint:
the operation is not visible in the URL.

### 5. Architecture and calculation history

**Prompts:**

> Actúa como un Principal Software Engineer especialista en FinTech. Basándote en
> la consigna anterior, propone una arquitectura limpia y modular para el
> proyecto. Separa el frontend (React/TypeScript) del backend (Go). Explícame cómo
> estructurar las carpetas, la API REST para registrar el historial de cálculos, y
> qué patrones de diseño podríamos aplicar.


_In English: as a Principal Engineer specialised in FinTech, propose a clean,
modular architecture; explain the folder structure, the REST API to record the
calculation history, and which design patterns apply. 

**Decision taken:** The assistant proposed a service layer and a repository
interface, and asked whether to store the history in memory or in a database.
I chose in-memory to keep it simple: the project stays
runnable with one command, and the interface leaves the door open for a
database.

**Result and iteration:** Calculations became a resource
(`POST` and `GET /api/v1/calculations`) with an append-only history and cursor
pagination. The tape in the UI now reads from the server. The limits of the
in-memory choice (history lost on restart, one history shared by every client)
are documented.

### 6. Decimal precision

**Prompts:**

> que libreria estas usando para los calculos? en las empresas de fintech, la
> precision decimal es muy importante

_In English: which library are you using for the calculations? In fintech
companies decimal precision is very important.

**Decision taken:** This was my challenge to the first version, which used
`float64`. The assistant confirmed the problem (`0.1 + 0.2` returned
`0.30000000000000004` from the API), compared three decimal libraries and
proposed `cockroachdb/apd` with 34 significant digits, half-even rounding and
numbers sent as strings. I approved that proposal.

**Result and iteration:** Exact decimal arithmetic end to end: the frontend
never converts an operand to a JavaScript number. `0 ^ 0` is reported as
undefined rather than `1`. Before fixing the numeric limits, the worst-case
exponentiations were timed to rule out slow requests.

### 7. Backend QA review

**Prompt:**

> ahora hace como si fueses un QA de backend. Revisa la lógica actual de la
> calculadora e identifica posibles edge cases o fallos (ej. división por cero,
> números muy grandes, entradas inválidas, etc). Agregá métodos de validación
> adecuados.

_In English: now act as a backend QA engineer. Review the calculator's current
logic, identify possible edge cases or failures (division by zero, very large
numbers, invalid input, and so on), and add appropriate validation._

**Decision taken:** I asked for an adversarial review instead of trusting the
existing tests, and accepted every fix, including the stricter ones that change
behaviour for clients: a `Content-Type` is now required and a second operand
sent to a unary operation is an error.

**Result and iteration:** Probing the running API found eight defects. Among
them: duplicate JSON keys were accepted, with the last one winning; a
percentage near the upper limit failed although its result was valid; a result
too close to zero returned the wrong error code. All eight were fixed,
validation was centralised, and fuzz tests were added. The full list is under
"Edge cases" in the README.

### 8. Frontend QA review

**Prompts:**

> bueno ahora que esta el back vamos con el front

_In English: now that the backend is done, let's do the frontend.

**Decision taken:** The same adversarial review for the UI. The second message
is a usability problem I ran into myself: in development the backend's root URL
showed a bare 404, which an evaluator would hit too.

**Result and iteration:** The review found a serious defect: keys typed while
a calculation was in flight were silently dropped, so typing `2 + 3 = × 4 =`
quickly showed `4` instead of `20`. Key presses are now queued and applied in
order. It also added a request timeout, validated paste and stricter checks on
responses. The backend root now redirects to the API documentation.

### 9. Sezzle Mode

**Prompts:**

> que pensas de hacer algo mas divertivo e innovador? Podriamos tener 2
> calculadores distintas: una normal, para las operaciones que hicimos hasta
> recien y otra que sea especifica de Sezzle, que te calcule las cuotas,
> especifica para la empresa


> le pondría "Normal Mode" y "Seezle Mode"

_In English: what do you think about something more fun and innovative? We
could have two calculators: a normal one, for the operations built so far, and
one specific to Sezzle that calculates the installments. Then: OK, let's go
with that. Then: I would call them "Normal Mode" and "Sezzle Mode"

**Decision taken:** The idea of a second, company-specific calculator and the
names of the two modes were mine. The assistant researched how Sezzle's plans
work in public sources and proposed limiting the scope to the three
interest-free plans, leaving out monthly financing and fees because their rules
could not be verified. I agreed with that scope.

**Result and iteration:** A payment planner that splits a purchase into 2, 4
or 5 installments, exact to the cent. Money is modelled as integer cents, a
separate type from the calculator's decimals. When an amount does not divide
evenly (`$99.99` in four), the leftover cents go on the first installment so
the remaining payments are equal. That rule, the 14-day spacing with no
weekend adjustment and the maximum amount are assumptions, and the README says
so. Tests caught two defects before release: dates were serialised with a time
and a zone, and a decorative `$` was part of a form field's accessible name.

### 10. Documentation

**Prompt:**

> Ahora armemos el read me y el promt. Ponele el formato al prompst similar a esto
>
> _(followed by a template for this log)_

_In English: now let's put together the README and the prompt log; format the
prompts like this template._

**Decision taken:** I supplied the structure of this document. The content of
each entry was written from what actually happened in the session, not from the
template's sample text.

**Result and iteration:** This log, and a README reorganised with screenshots
and a section listing every edge case and its behaviour.
