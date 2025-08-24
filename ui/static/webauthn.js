/**
 * @param base64String {string}
 * @returns {Uint8Array}
 */
function bufferDecode(base64String) {
  const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/");

  const rawData = window.atob(base64);
  const outputArray = new Uint8Array(rawData.length);

  for (let i = 0; i < rawData.length; ++i) {
    outputArray[i] = rawData.charCodeAt(i);
  }

  return outputArray;
}

/**
 * @param value {ArrayBuffer}
 * @returns {string}
 */
function bufferEncode(value) {
  return btoa(String.fromCharCode.apply(null, new Uint8Array(value)))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=/g, "");
}

/**
 * Submits given form and decodes JSON response.
 * @param form {HTMLFormElement}
 * @returns {Promise<any>}
 */
async function submitForm(form) {
  const url = form.action
  const resp = await fetch(url, {method: "post"})

  if (!resp.ok) {
    throw new Error(`Failed to submit form!`);
  }

  return resp.json()
}

/**
 * Creates Webauthn attestation response to be sent to finish registration endpoint.
 * @param publicKey is the publicKey field in response from the start registration endpoint.
 * @returns {Promise<string>}
 */
async function createAttestationResponse(publicKey) {
  publicKey.challenge = bufferDecode(/** @type {string} */ publicKey.challenge);
  publicKey.user.id = bufferDecode(/** @type {string} */ publicKey.user.id);
  publicKey.excludeCredentials = publicKey.excludeCredentials?.map((excludedCredential) => ({
    ...excludedCredential,
    id: bufferDecode(excludedCredential.id),
  }))
  const credential = await navigator.credentials.create({publicKey});
  const {id, rawId, type, response: {attestationObject, clientDataJSON}} = credential;
  return JSON.stringify({
    id,
    rawId: bufferEncode(rawId),
    type,
    response: {
      attestationObject: bufferEncode(attestationObject),
      clientDataJSON: bufferEncode(clientDataJSON),
    },
  })
}

/**
 * Finishes registration with the server and reloads the page.
 * @param attestationResponse is the payload sent to the finish registration endpoint.
 * @returns {Promise<void>}
 */
async function finishRegistration(attestationResponse) {
  const finishResp = await fetch("/api/registration/finish", {method: "post", body: attestationResponse})
  if (!finishResp.ok) {
    throw new Error("Finishing registration failed!");
  }
  // At this point, we assume the cookies are in place so that we can reload the page with the proper access.
  window.location.reload();
}

/**
 * Registers a user using Webauthn.
 * @param e {SubmitEvent}
 */
export async function registerUser(e) {
  try {
    e.preventDefault()
    const credentialCreationOptions = await submitForm(e.target)
    const attestationResponse = await createAttestationResponse(credentialCreationOptions.publicKey)
    await finishRegistration(attestationResponse)
  } catch (err) {
    console.error(err)
    throw new Error("Registration failed!");
  }
}

/**
 * Creates Webauthn assertion response to be sent to finish login endpoint.
 * @param publicKey is the publicKey field in response from the start login endpoint.
 * @returns {Promise<string>}
 */
async function createAssertionResponse(publicKey) {
  publicKey.challenge = bufferDecode(/** @type {string} */ publicKey.challenge);
  const assertion = await navigator.credentials.get({publicKey});
  const {id, rawId, type, response: {authenticatorData, clientDataJSON, signature, userHandle}} = assertion;
  return JSON.stringify({
    id,
    rawId: bufferEncode(rawId),
    type,
    response: {
      authenticatorData: bufferEncode(authenticatorData),
      clientDataJSON: bufferEncode(clientDataJSON),
      signature: bufferEncode(signature),
      userHandle: bufferEncode(userHandle),
    },
  })
}

/**
 * Finishes login with the server and reloads the page.
 * @param assertionResponse is the payload sent to the finish login endpoint.
 * @returns {Promise<void>}
 */
async function finishLogin(assertionResponse) {
  const finishResp = await fetch("/api/login/finish", {method: "post", body: assertionResponse})
  if (!finishResp.ok) {
    throw new Error("Finishing login failed!");
  }
  // At this point, we assume the cookies are in place so that we can reload the page with the proper access.
  window.location.reload();
}

/**
 * Logs in a user using Webauthn.
 * @param e {SubmitEvent}
 */
export async function loginUser(e) {
  e.preventDefault()
  try {
    const credentialRequestOptions = await submitForm(e.target)
    const assertionResponse = await createAssertionResponse(credentialRequestOptions.publicKey)
    await finishLogin(assertionResponse)
  } catch (err) {
    console.error(err)
    throw new Error("Login failed!");
  }
}
